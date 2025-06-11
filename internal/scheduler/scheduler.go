// internal/scheduler/scheduler.go
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github-service/internal/service"
	"github-service/pkg/logger"
	"github-service/pkg/utils"
)

// Scheduler handles scheduled tasks
type Scheduler struct {
	githubService *service.GitHubService
	logger        logger.Logger
}

// NewScheduler creates a new scheduler
func NewScheduler(githubService *service.GitHubService, logger logger.Logger) *Scheduler {
	return &Scheduler{
		githubService: githubService,
		logger:        logger,
	}
}

// Start starts the scheduler
func (s *Scheduler) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	s.logger.Info("Starting scheduler", "interval", interval)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Scheduler stopped")
			return
		case <-ticker.C:
			s.runScheduledSync(ctx)
		}
	}
}

// runScheduledSync runs a single sync operation with retry
func (s *Scheduler) runScheduledSync(ctx context.Context) {
	s.logger.Info("Starting scheduled sync")

	// Use the generic retry utility for the entire sync operation
	err := utils.WithBackoff(ctx, utils.Config{
		MaxRetries: 3,
		MinDelay:   5 * time.Second,
		MaxDelay:   5 * time.Minute,
		ShouldRetry: func(err error) bool {
			// Don't retry context cancellations
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return false
			}
			
			// Log the error and retry
			s.logger.Error("Sync failed, will retry", "error", err)
			return true
		},
	}, func() error {
		// Create a new context with timeout for this attempt
		ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()

		// Start a transaction
		tx, err := s.githubService.TxManager.BeginTransaction(ctx, "scheduled_sync")
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}

		// Ensure we always rollback on error
		defer func() {
			// if tx != nil {
				if rbErr := tx.Rollback(); rbErr != nil {
					s.logger.Error("Failed to rollback transaction", "error", rbErr)
				}
			// }
		}()

		// Run the sync
		if err := s.githubService.SyncAllRepositories(ctx); err != nil {
			return fmt.Errorf("sync failed: %w", err)
		}

		// Commit the transaction
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}

		return nil
	})

	if err != nil {
		s.logger.Error("Scheduled sync failed after retries", "error", err)
	} else {
		s.logger.Info("Scheduled sync completed successfully")
	}
}