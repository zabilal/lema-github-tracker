package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github-service/internal/github"
	"github-service/internal/models"
	"github-service/internal/repository"
	"github-service/internal/workers"
	"github-service/pkg/logger"
	"github-service/pkg/utils"
)

type GitHubService struct {
	githubClient     github.GitHubClient
	repository       *repository.Repository
	logger           logger.Logger
	rateLimitHandler *github.RateLimitHandler
	TxManager        repository.TransactionManager
	batchSize        int
	maxRetries       int
	retryDelay       time.Duration
	workerPool       *workers.Pool

}

func NewGitHubService(
	githubClient github.GitHubClient,
	repo *repository.Repository,
	logger logger.Logger,
	rateLimitHandler *github.RateLimitHandler,
	txManager repository.TransactionManager,
	workerCount int,
) *GitHubService {
	return &GitHubService{
		githubClient:     githubClient,
		repository:       repo,
		logger:           logger,
		rateLimitHandler: rateLimitHandler,
		TxManager:        txManager,
		batchSize:  100,
		maxRetries: 3,
		retryDelay: time.Second * 2,
		workerPool: workers.NewPool(workerCount),
	}
}

// SyncRepository syncs a repository with GitHub
func (s *GitHubService) SyncRepository(ctx context.Context, tx repository.Transaction, owner, repoName string) error {
	// Use the rate limit handler for GitHub API calls
	err := s.rateLimitHandler.RetryOperation(ctx, func() error {
		return s.FetchAndStoreRepository(ctx, tx, owner, repoName)
	})
	
	if err != nil {
		return fmt.Errorf("failed to sync repository: %w", err)
	}
	
	return nil
}

// syncRepositoryWithRetry syncs a repository with retry logic
func (s *GitHubService) syncRepositoryWithRetry(ctx context.Context, tx repository.Transaction, owner, repoName string) error {
	// Use the generic retry utility for non-GitHub operations
	return utils.WithBackoff(ctx, utils.Config{
		MaxRetries: s.maxRetries,
		MinDelay:   s.retryDelay,
		MaxDelay:   30 * time.Second,
		ShouldRetry: func(err error) bool {
			// Don't retry context cancellations
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return false
			}
			
			// Retry on any other error
			return true
		},
	}, func() error {
		// Actual sync logic here
		_, err := s.githubClient.GetRepository(ctx, owner, repoName)
		if err != nil {
			return fmt.Errorf("failed to get repository: %w", err)
		}

		// Save the repository to the database
		// if err := s.repository.SaveRepository(ctx, tx, repo); err != nil {
		// 	return fmt.Errorf("failed to save repository: %w", err)
		// }

		return nil
	})
}

func (s *GitHubService) FetchAndStoreRepository(ctx context.Context, tx repository.Transaction, owner, repoName string) error {
	fullName := fmt.Sprintf("%s/%s", owner, repoName)

	s.logger.Info("Fetching repository", "repository", fullName)

	// Fetch repository from GitHub
	var githubRepo *models.Repository
    err := s.rateLimitHandler.RetryOperation(ctx, func() error {
        var err error
        githubRepo, err = s.githubClient.GetRepository(ctx, owner, repoName)
        return err
    })

	if err != nil {
		s.logger.Error("Failed to fetch repository", "repository", fullName, "error", err)
		return fmt.Errorf("failed to fetch repository: %w", err)
	}

	// Check if repository exists in database
	existingRepo, err := s.repository.GetRepositoryByFullName(fullName)
	if err != nil {
		return fmt.Errorf("failed to check existing repository: %w", err)
	}

	var dbRepo *models.Repository
	if existingRepo == nil {
		// Create new repository
		dbRepo = &models.Repository{
			Name:            githubRepo.Name,
			FullName:        githubRepo.FullName,
			Description:     githubRepo.Description,
			URL:             githubRepo.URL,
			Language:        githubRepo.Language,
			ForksCount:      githubRepo.ForksCount,
			StarsCount:      githubRepo.StarsCount,
			OpenIssuesCount: githubRepo.OpenIssuesCount,
			WatchersCount:   githubRepo.WatchersCount,
			CreatedAt:       githubRepo.CreatedAt,
			UpdatedAt:       githubRepo.UpdatedAt,
			SyncSince:       time.Now().AddDate(0, 0, -30), // Default to 30 days ago
		}

		if err := s.repository.CreateRepository(tx, dbRepo); err != nil {
			return fmt.Errorf("failed to create repository: %w", err)
		}
		s.logger.Info("Created new repository", "repository", fullName, "id", dbRepo.ID)
	} else {
		// Update existing repository
		dbRepo = existingRepo
		dbRepo.Name = githubRepo.Name
		dbRepo.Description = githubRepo.Description
		dbRepo.URL = githubRepo.URL
		dbRepo.Language = githubRepo.Language
		dbRepo.ForksCount = githubRepo.ForksCount
		dbRepo.StarsCount = githubRepo.StarsCount
		dbRepo.OpenIssuesCount = githubRepo.OpenIssuesCount
		dbRepo.WatchersCount = githubRepo.WatchersCount
		dbRepo.UpdatedAt = githubRepo.UpdatedAt

		if err := s.repository.UpdateRepository(tx, dbRepo); err != nil {
			return fmt.Errorf("failed to update repository: %w", err)
		}
		s.logger.Info("Updated existing repository", "repository", fullName, "id", dbRepo.ID)
	}

	// Fetch and store commits
	if err := s.fetchAndStoreCommits(ctx, tx, owner, repoName, dbRepo); err != nil {
		return fmt.Errorf("failed to fetch and store commits: %w", err)
	}

	return nil
}

func (s *GitHubService) fetchAndStoreCommits(ctx context.Context, tx repository.Transaction, owner, repoName string, repo *models.Repository) error {
	s.logger.Info("Fetching commits", "repository", repo.FullName, "since", repo.SyncSince)

	page := 1
	perPage := 100
	totalCommits := 0
	newCommits := 0

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		commits, err := s.githubClient.GetCommits(ctx, owner, repoName, &repo.SyncSince, page, perPage)
		if err != nil {
			return fmt.Errorf("failed to fetch commits from GitHub: %w", err)
		}

		if len(commits) == 0 {
			break
		}

		for _, githubCommit := range commits {
			// Check if commit already exists
			exists, err := s.repository.CommitExists(tx, repo.ID, githubCommit.SHA)
			if err != nil {
				return fmt.Errorf("failed to check if commit exists: %w", err)
			}

			if !exists {
				commit := &models.Commit{
					RepositoryID: repo.ID,
					SHA:          githubCommit.SHA,
					Message:      githubCommit.Commit.Message,
					AuthorName:   githubCommit.Commit.Author.Name,
					AuthorEmail:  githubCommit.Commit.Author.Email,
					CommitDate:   githubCommit.Commit.Author.Date,
					URL:          githubCommit.HTMLURL,
				}

				if err := s.repository.CreateCommit(tx, commit); err != nil {
					return fmt.Errorf("failed to create commit: %w", err)
				}
				newCommits++
			}
			totalCommits++
		}

		// Update repository with latest commit SHA
		if len(commits) > 0 {
			repo.LastCommitSHA = &commits[0].SHA
			now := time.Now()
			repo.LastSyncedAt = &now
			if err := s.repository.UpdateRepository(tx, repo); err != nil {
				s.logger.Error("Failed to update repository last commit SHA", "error", err)
			}
		}

		// If we got fewer commits than requested, we've reached the end
		if len(commits) < perPage {
			break
		}

		page++
	}

	s.logger.Info("Finished fetching commits",
		"repository", repo.FullName,
		"total_processed", totalCommits,
		"new_commits", newCommits)

	return nil
}

// SyncAllRepositories processes all repositories concurrently
func (s *GitHubService) SyncAllRepositories(ctx context.Context) error {
	// Get all repositories to sync
	repos, err := s.repository.GetAllRepositories(1, s.batchSize)
	if err != nil {
		return fmt.Errorf("failed to fetch all repositories: %w", err)
	}

	// Create a wait group for batch processing
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, s.batchSize) // Limit concurrent operations

	for _, repo := range repos {
		wg.Add(1)
		semaphore <- struct{}{} // Acquire semaphore

		parts := strings.Split(repo.FullName, "/")
		if len(parts) != 2 {
			s.logger.Error("Invalid repository full name", "full_name", repo.FullName)
			continue
		}

		owner, repoName := parts[0], parts[1]	

		go func(r models.Repository) {
			defer wg.Done()
			defer func() { <-semaphore }() // Release semaphore

			// Check rate limits before starting
			if err := s.rateLimitHandler.WaitIfNeeded(ctx); err != nil {
				s.logger.Error("Rate limit wait failed", "error", err)
				return
			}

			// Process repository
			err := s.TxManager.WithTransaction(ctx, "sync_repository", func(tx repository.Transaction) error {
				return s.SyncRepository(ctx, tx, owner, repoName)
			})

			if err != nil {
				s.logger.Error("Failed to sync repository",
					"owner", owner,
					"repo", repoName,
					"error", err,
				)
			}
		}(repo)
	}

	// Wait for all repositories to be processed
	wg.Wait()
	return nil
}

// SyncAllRepositories syncs all repositories in batches to prevent memory issues
// It processes repositories in pages of 50 items by default
// func (s *GitHubService) SyncAllRepositories(ctx context.Context, tx repository.Transaction) error {
// 	page := 1
// 	pageSize := 50

// 	for {
// 		repositories, err := s.repository.GetAllRepositories(page, pageSize)
// 		if err != nil {
// 			return fmt.Errorf("failed to get repositories (page %d): %w", page, err)
// 		}

// 		// If no more repositories, we're done
// 		if len(repositories) == 0 {
// 			break
// 		}

// 		for _, repo := range repositories {
// 			select {
// 			case <-ctx.Done():
// 				return ctx.Err()
// 			default:
// 				parts := strings.Split(repo.FullName, "/")
// 				if len(parts) != 2 {
// 					s.logger.Error("Invalid repository full name", "full_name", repo.FullName)
// 					continue
// 				}

// 				owner, repoName := parts[0], parts[1]
// 				s.logger.Info("Syncing repository", "repository", repo.FullName, "page", page)

// 				if err := s.FetchAndStoreRepository(ctx, tx, owner, repoName); err != nil {
// 					s.logger.Error("Failed to sync repository",
// 						"repository", repo.FullName,
// 						"error", err,
// 						"page", page)
// 					continue
// 				}
// 			}
// 		}

// 		// If we got fewer items than the page size, this was the last page
// 		if len(repositories) < pageSize {
// 			break
// 		}

// 		page++
// 	}

// 	return nil
// }

func (s *GitHubService) ResetRepositorySync(fullName string, since time.Time) error {
	return s.repository.ResetRepositorySyncSince(fullName, since)
}

func (s *GitHubService) GetTopCommitAuthors(limit int) ([]models.CommitAuthorStats, error) {
	return s.repository.GetTopCommitAuthors(limit)
}

func (s *GitHubService) GetCommitsByRepository(repositoryName string, page, pageSize int) ([]models.Commit, error) {
	return s.repository.GetCommitsByRepository(repositoryName, page, pageSize)
}

func (s *GitHubService) GetRepositoryStats(repositoryName string) (*models.RepositoryStatsResponse, error) {
	repo, err := s.repository.GetRepositoryByFullName(repositoryName)
	if err != nil {
		return nil, err
	}

	commits, err := s.repository.GetCommitCountByRepository(repo.ID)
	if err != nil {
		return nil, err
	}
	return &models.RepositoryStatsResponse{
		Name:            repo.Name,
		FullName:        repo.FullName,
		Description:     *repo.Description,
		Language:        repo.Language,
		StarsCount:      repo.StarsCount,
		ForksCount:      repo.ForksCount,
		OpenIssuesCount: repo.OpenIssuesCount,
		WatchersCount:   repo.WatchersCount,
		TotalCommits:    commits[0].CommitCount,
		LastSyncedAt:    repo.LastSyncedAt,
		CreatedAt:       repo.CreatedAt,
		UpdatedAt:       repo.UpdatedAt,
	}, nil
}
