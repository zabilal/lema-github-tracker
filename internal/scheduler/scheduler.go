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
			if err := s.githubService.SyncAllRepositories(ctx); err != nil {
				s.logger.Error("Failed to sync repositories", "error", err)
			} else {
				s.logger.Info("Scheduled sync completed")
			}
		}
	}
}
