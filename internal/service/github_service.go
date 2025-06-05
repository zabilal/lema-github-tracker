package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github-service/internal/github"
	"github-service/internal/models"
	"github-service/internal/repository"
	"github-service/pkg/logger"
)

type GitHubService struct {
	githubClient *github.Client
	repository   *repository.Repository
	logger       logger.Logger
}

func NewGitHubService(githubClient *github.Client, repo *repository.Repository, logger logger.Logger) *GitHubService {
	return &GitHubService{
		githubClient: githubClient,
		repository:   repo,
		logger:       logger,
	}
}

func (s *GitHubService) FetchAndStoreRepository(ctx context.Context, owner, repoName string) error {
	fullName := fmt.Sprintf("%s/%s", owner, repoName)

	s.logger.Info("Fetching repository", "repository", fullName)

	// Fetch repository from GitHub
	githubRepo, err := s.githubClient.GetRepository(owner, repoName)
	if err != nil {
		return fmt.Errorf("failed to fetch repository from GitHub: %w", err)
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
			URL:             githubRepo.HTMLURL,
			Language:        githubRepo.Language,
			ForksCount:      githubRepo.ForksCount,
			StarsCount:      githubRepo.StargazersCount,
			OpenIssuesCount: githubRepo.OpenIssuesCount,
			WatchersCount:   githubRepo.WatchersCount,
			CreatedAt:       githubRepo.CreatedAt,
			UpdatedAt:       githubRepo.UpdatedAt,
			SyncSince:       time.Now().AddDate(0, 0, -30), // Default to 30 days ago
		}

		if err := s.repository.CreateRepository(dbRepo); err != nil {
			return fmt.Errorf("failed to create repository: %w", err)
		}
		s.logger.Info("Created new repository", "repository", fullName, "id", dbRepo.ID)
	} else {
		// Update existing repository
		dbRepo = existingRepo
		dbRepo.Name = githubRepo.Name
		dbRepo.Description = githubRepo.Description
		dbRepo.URL = githubRepo.HTMLURL
		dbRepo.Language = githubRepo.Language
		dbRepo.ForksCount = githubRepo.ForksCount
		dbRepo.StarsCount = githubRepo.StargazersCount
		dbRepo.OpenIssuesCount = githubRepo.OpenIssuesCount
		dbRepo.WatchersCount = githubRepo.WatchersCount
		dbRepo.UpdatedAt = githubRepo.UpdatedAt

		if err := s.repository.UpdateRepository(dbRepo); err != nil {
			return fmt.Errorf("failed to update repository: %w", err)
		}
		s.logger.Info("Updated existing repository", "repository", fullName, "id", dbRepo.ID)
	}

	// Fetch and store commits
	if err := s.fetchAndStoreCommits(ctx, owner, repoName, dbRepo); err != nil {
		return fmt.Errorf("failed to fetch and store commits: %w", err)
	}

	return nil
}

func (s *GitHubService) fetchAndStoreCommits(ctx context.Context, owner, repoName string, repo *models.Repository) error {
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

		commits, err := s.githubClient.GetCommits(owner, repoName, repo.SyncSince, page, perPage)
		if err != nil {
			return fmt.Errorf("failed to fetch commits from GitHub: %w", err)
		}

		if len(commits) == 0 {
			break
		}

		for _, githubCommit := range commits {
			// Check if commit already exists
			exists, err := s.repository.CommitExists(repo.ID, githubCommit.SHA)
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

				if err := s.repository.CreateCommit(commit); err != nil {
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
			if err := s.repository.UpdateRepository(repo); err != nil {
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

// SyncAllRepositories syncs all repositories in batches to prevent memory issues
// It processes repositories in pages of 50 items by default
func (s *GitHubService) SyncAllRepositories(ctx context.Context) error {
	page := 1
	pageSize := 50

	for {
		repositories, err := s.repository.GetAllRepositories(page, pageSize)
		if err != nil {
			return fmt.Errorf("failed to get repositories (page %d): %w", page, err)
		}

		// If no more repositories, we're done
		if len(repositories) == 0 {
			break
		}

		for _, repo := range repositories {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				parts := strings.Split(repo.FullName, "/")
				if len(parts) != 2 {
					s.logger.Error("Invalid repository full name", "full_name", repo.FullName)
					continue
				}

				owner, repoName := parts[0], parts[1]
				s.logger.Info("Syncing repository", "repository", repo.FullName, "page", page)

				if err := s.FetchAndStoreRepository(ctx, owner, repoName); err != nil {
					s.logger.Error("Failed to sync repository",
						"repository", repo.FullName,
						"error", err,
						"page", page)
					continue
				}
			}
		}

		// If we got fewer items than the page size, this was the last page
		if len(repositories) < pageSize {
			break
		}

		page++
	}

	return nil
}

func (s *GitHubService) ResetRepositorySync(fullName string, since time.Time) error {
	return s.repository.ResetRepositorySyncSince(fullName, since)
}

func (s *GitHubService) GetTopCommitAuthors(limit int) ([]models.CommitAuthorStats, error) {
	return s.repository.GetTopCommitAuthors(limit)
}

func (s *GitHubService) GetCommitsByRepository(repositoryName string, page, pageSize int) ([]models.Commit, error) {
	return s.repository.GetCommitsByRepository(repositoryName, page, pageSize)
}
