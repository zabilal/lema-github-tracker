package scheduler

import (
	"context"
	"time"

	"github-service/internal/service"
	"github-service/pkg/logger"
)

type Scheduler struct {
	githubService *service.GitHubService
	logger        logger.Logger
}

func New(githubService *service.GitHubService, logger logger.Logger) *Scheduler {
	return &Scheduler{
		githubService: githubService,
		logger:        logger,
	}
}

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
            s.logger.Info("Starting scheduled sync")
            
            // Create a new transaction for this sync operation
            tx, err := s.githubService.TxManager.BeginTransaction(ctx, "sync_all_repositories")
            if err != nil {
                s.logger.Error("Failed to begin transaction", "error", err)
                continue
            }

            // Pass both context and transaction
            if err := s.githubService.SyncAllRepositories(ctx, tx); err != nil {
                s.logger.Error("Failed to sync repositories", "error", err)
                if rbErr := tx.Rollback(); rbErr != nil {
                    s.logger.Error("Failed to rollback transaction", "error", rbErr)
                }
                continue
            }

            // Commit the transaction if everything went well
            if err := tx.Commit(); err != nil {
                s.logger.Error("Failed to commit transaction", "error", err)
                continue
            }
            
            s.logger.Info("Scheduled sync completed")
        }
    }
}