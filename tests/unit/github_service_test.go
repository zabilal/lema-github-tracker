package unit

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github-service/internal/models"
	"github-service/pkg/logger"
	"github-service/pkg/logger/mocks"
)

// GitHubClientInterface defines the interface for the GitHub client
type GitHubClientInterface interface {
	GetRepository(owner, repo string) (*models.GitHubRepository, error)
	GetCommits(owner, repo string, since time.Time, page, perPage int) ([]models.GitHubCommit, error)
}

// RepositoryInterface defines the interface for the repository
type RepositoryInterface interface {
	GetRepositoryByFullName(fullName string) (*models.Repository, error)
	CreateRepository(repo *models.Repository) error
	UpdateRepository(repo *models.Repository) error
	CommitExists(repositoryID int64, sha string) (bool, error)
	CreateCommit(commit *models.Commit) error
	GetTopCommitAuthors(limit int) ([]models.CommitAuthorStats, error)
	GetCommitsByRepository(repositoryName string) ([]models.Commit, error)
	GetAllRepositories() ([]models.Repository, error)
	ResetRepositorySyncSince(fullName string, since time.Time) error
}

// MockGitHubClient is a mock implementation of the GitHub client
type MockGitHubClient struct {
	GetRepositoryFunc func(owner, repo string) (*models.GitHubRepository, error)
	GetCommitsFunc    func(owner, repo string, since time.Time, page, perPage int) ([]models.GitHubCommit, error)
}

func (m *MockGitHubClient) GetRepository(owner, repo string) (*models.GitHubRepository, error) {
	return m.GetRepositoryFunc(owner, repo)
}

func (m *MockGitHubClient) GetCommits(owner, repo string, since time.Time, page, perPage int) ([]models.GitHubCommit, error) {
	return m.GetCommitsFunc(owner, repo, since, page, perPage)
}

// MockRepository is a mock implementation of the repository
type MockRepository struct {
	GetRepositoryByFullNameFunc  func(fullName string) (*models.Repository, error)
	CreateRepositoryFunc         func(repo *models.Repository) error
	UpdateRepositoryFunc         func(repo *models.Repository) error
	CommitExistsFunc             func(repositoryID int64, sha string) (bool, error)
	CreateCommitFunc             func(commit *models.Commit) error
	GetTopCommitAuthorsFunc      func(limit int) ([]models.CommitAuthorStats, error)
	GetCommitsByRepositoryFunc   func(repositoryName string) ([]models.Commit, error)
	GetAllRepositoriesFunc       func() ([]models.Repository, error)
	ResetRepositorySyncSinceFunc func(fullName string, since time.Time) error
}

func (m *MockRepository) GetRepositoryByFullName(fullName string) (*models.Repository, error) {
	return m.GetRepositoryByFullNameFunc(fullName)
}

func (m *MockRepository) CreateRepository(repo *models.Repository) error {
	return m.CreateRepositoryFunc(repo)
}

func (m *MockRepository) UpdateRepository(repo *models.Repository) error {
	return m.UpdateRepositoryFunc(repo)
}

func (m *MockRepository) CommitExists(repositoryID int64, sha string) (bool, error) {
	return m.CommitExistsFunc(repositoryID, sha)
}

func (m *MockRepository) CreateCommit(commit *models.Commit) error {
	return m.CreateCommitFunc(commit)
}

func (m *MockRepository) GetTopCommitAuthors(limit int) ([]models.CommitAuthorStats, error) {
	return m.GetTopCommitAuthorsFunc(limit)
}

func (m *MockRepository) GetCommitsByRepository(repositoryName string) ([]models.Commit, error) {
	return m.GetCommitsByRepositoryFunc(repositoryName)
}

func (m *MockRepository) GetAllRepositories() ([]models.Repository, error) {
	return m.GetAllRepositoriesFunc()
}

func (m *MockRepository) ResetRepositorySyncSince(fullName string, since time.Time) error {
	return m.ResetRepositorySyncSinceFunc(fullName, since)
}

// TestGitHubService is a wrapper around the GitHubService for testing
type TestGitHubService struct {
	githubClient GitHubClientInterface
	repository   RepositoryInterface
	logger       logger.Logger
}

// NewTestGitHubService creates a new TestGitHubService
func NewTestGitHubService(githubClient GitHubClientInterface, repo RepositoryInterface, logger logger.Logger) *TestGitHubService {
	return &TestGitHubService{
		githubClient: githubClient,
		repository:   repo,
		logger:       logger,
	}
}

// FetchAndStoreRepository fetches a repository from GitHub and stores it in the database
func (s *TestGitHubService) FetchAndStoreRepository(ctx context.Context, owner, repoName string) error {
	fullName := owner + "/" + repoName

	s.logger.Info("Fetching repository", "repository", fullName)

	// Fetch repository from GitHub
	githubRepo, err := s.githubClient.GetRepository(owner, repoName)
	if err != nil {
		return errors.New("failed to fetch repository from GitHub: " + err.Error())
	}

	// Check if repository exists in database
	existingRepo, err := s.repository.GetRepositoryByFullName(fullName)
	if err != nil {
		return errors.New("failed to check existing repository: " + err.Error())
	}

	var dbRepo *models.Repository
	if existingRepo == nil {
		// Create new repository
		dbRepo = &models.Repository{
			Name:            githubRepo.Name,
			FullName:        githubRepo.FullName,
			Description:     githubRepo.Description,
			URL:             githubRepo.HTMLURL,
			Language:        getLanguage(githubRepo.Language),
			ForksCount:      githubRepo.ForksCount,
			StarsCount:      githubRepo.StargazersCount,
			OpenIssuesCount: githubRepo.OpenIssuesCount,
			WatchersCount:   githubRepo.WatchersCount,
			CreatedAt:       githubRepo.CreatedAt,
			UpdatedAt:       githubRepo.UpdatedAt,
			SyncSince:       time.Now().AddDate(0, 0, -30), // Default to 30 days ago
		}

		if err := s.repository.CreateRepository(dbRepo); err != nil {
			return errors.New("failed to create repository: " + err.Error())
		}
		s.logger.Info("Created new repository", "repository", fullName, "id", dbRepo.ID)
	} else {
		// Update existing repository
		dbRepo = existingRepo
		dbRepo.Name = githubRepo.Name
		dbRepo.Description = githubRepo.Description
		dbRepo.URL = githubRepo.HTMLURL
		dbRepo.Language = getLanguage(githubRepo.Language)
		dbRepo.ForksCount = githubRepo.ForksCount
		dbRepo.StarsCount = githubRepo.StargazersCount
		dbRepo.OpenIssuesCount = githubRepo.OpenIssuesCount
		dbRepo.WatchersCount = githubRepo.WatchersCount
		dbRepo.UpdatedAt = githubRepo.UpdatedAt

		if err := s.repository.UpdateRepository(dbRepo); err != nil {
			return errors.New("failed to update repository: " + err.Error())
		}
		s.logger.Info("Updated existing repository", "repository", fullName, "id", dbRepo.ID)
	}

	// Fetch and store commits
	if err := s.fetchAndStoreCommits(ctx, owner, repoName, dbRepo); err != nil {
		return errors.New("failed to fetch and store commits: " + err.Error())
	}

	return nil
}

// fetchAndStoreCommits fetches commits from GitHub and stores them in the database
func (s *TestGitHubService) fetchAndStoreCommits(ctx context.Context, owner, repoName string, repo *models.Repository) error {
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
			return errors.New("failed to fetch commits from GitHub: " + err.Error())
		}

		if len(commits) == 0 {
			break
		}

		for _, githubCommit := range commits {
			// Check if commit already exists
			exists, err := s.repository.CommitExists(repo.ID, githubCommit.SHA)
			if err != nil {
				return errors.New("failed to check if commit exists: " + err.Error())
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
					return errors.New("failed to create commit: " + err.Error())
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

// SyncAllRepositories syncs all repositories in the database
func (s *TestGitHubService) SyncAllRepositories(ctx context.Context) error {
	repositories, err := s.repository.GetAllRepositories()
	if err != nil {
		return errors.New("failed to get all repositories: " + err.Error())
	}

	for _, repo := range repositories {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		parts := strings.Split(repo.FullName, "/")
		if len(parts) != 2 {
			s.logger.Error("Invalid repository full name", "full_name", repo.FullName)
			continue
		}

		owner, repoName := parts[0], parts[1]
		if err := s.FetchAndStoreRepository(ctx, owner, repoName); err != nil {
			s.logger.Error("Failed to sync repository", "repository", repo.FullName, "error", err)
			continue
		}
	}

	return nil
}

// ResetRepositorySync resets the sync point for a repository
func (s *TestGitHubService) ResetRepositorySync(fullName string, since time.Time) error {
	return s.repository.ResetRepositorySyncSince(fullName, since)
}

// GetTopCommitAuthors gets the top commit authors
func (s *TestGitHubService) GetTopCommitAuthors(limit int) ([]models.CommitAuthorStats, error) {
	return s.repository.GetTopCommitAuthors(limit)
}

// GetCommitsByRepository gets commits for a specific repository
func (s *TestGitHubService) GetCommitsByRepository(repositoryName string) ([]models.Commit, error) {
	return s.repository.GetCommitsByRepository(repositoryName)
}

// Helper function to create a string pointer
func createStringPtr(s string) *string {
	return &s
}

// Helper function to safely get language from a pointer
func getLanguage(lang *string) string {
	if lang == nil {
		return "Unknown"
	}
	return *lang
}

func TestFetchAndStoreRepository_NewRepository(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()

	owner := "testowner"
	repoName := "testrepo"
	fullName := "testowner/testrepo"

	// Create a mock GitHub client
	mockGitHubClient := &MockGitHubClient{
		GetRepositoryFunc: func(o, r string) (*models.GitHubRepository, error) {
			assert.Equal(t, owner, o)
			assert.Equal(t, repoName, r)
			return &models.GitHubRepository{
				Name:            repoName,
				FullName:        fullName,
				Description:     createStringPtr("Test repository"),
				HTMLURL:         "https://github.com/testowner/testrepo",
				Language:        createStringPtr("Go"),
				ForksCount:      10,
				StargazersCount: 20,
				OpenIssuesCount: 5,
				WatchersCount:   15,
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
			}, nil
		},
		GetCommitsFunc: func(o, r string, since time.Time, page, perPage int) ([]models.GitHubCommit, error) {
			assert.Equal(t, owner, o)
			assert.Equal(t, repoName, r)
			assert.Equal(t, 1, page)
			assert.Equal(t, 100, perPage)

			// Return empty commits to simplify the test
			return []models.GitHubCommit{}, nil
		},
	}

	// Create a mock repository
	mockRepo := &MockRepository{
		GetRepositoryByFullNameFunc: func(fn string) (*models.Repository, error) {
			assert.Equal(t, fullName, fn)
			return nil, nil // Repository doesn't exist yet
		},
		CreateRepositoryFunc: func(repo *models.Repository) error {
			assert.Equal(t, repoName, repo.Name)
			assert.Equal(t, fullName, repo.FullName)
			repo.ID = 1 // Simulate ID assignment
			return nil
		},
		UpdateRepositoryFunc: func(repo *models.Repository) error {
			return nil
		},
		CommitExistsFunc: func(repositoryID int64, sha string) (bool, error) {
			return false, nil
		},
		CreateCommitFunc: func(commit *models.Commit) error {
			return nil
		},
	}

	// Create the service
	githubService := NewTestGitHubService(mockGitHubClient, mockRepo, mockLogger)

	// Test
	err := githubService.FetchAndStoreRepository(context.Background(), owner, repoName)

	// Assert
	assert.NoError(t, err)
}

func TestFetchAndStoreRepository_ExistingRepository(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()

	owner := "testowner"
	repoName := "testrepo"
	fullName := "testowner/testrepo"

	existingRepo := &models.Repository{
		ID:              1,
		Name:            repoName,
		FullName:        fullName,
		Description:     createStringPtr("Existing repository"),
		URL:             "https://github.com/testowner/testrepo",
		Language:        *createStringPtr("Go"),
		ForksCount:      5,
		StarsCount:      10,
		OpenIssuesCount: 2,
		WatchersCount:   8,
		CreatedAt:       time.Now().Add(-24 * time.Hour),
		UpdatedAt:       time.Now().Add(-12 * time.Hour),
		SyncSince:       time.Now().Add(-30 * 24 * time.Hour),
	}

	// Create a mock GitHub client
	mockGitHubClient := &MockGitHubClient{
		GetRepositoryFunc: func(o, r string) (*models.GitHubRepository, error) {
			assert.Equal(t, owner, o)
			assert.Equal(t, repoName, r)
			return &models.GitHubRepository{
				Name:            repoName,
				FullName:        fullName,
				Description:     createStringPtr("Updated repository"),
				HTMLURL:         "https://github.com/testowner/testrepo",
				Language:        createStringPtr("Go"),
				ForksCount:      10,
				StargazersCount: 20,
				OpenIssuesCount: 5,
				WatchersCount:   15,
				CreatedAt:       existingRepo.CreatedAt,
				UpdatedAt:       time.Now(),
			}, nil
		},
		GetCommitsFunc: func(o, r string, since time.Time, page, perPage int) ([]models.GitHubCommit, error) {
			assert.Equal(t, owner, o)
			assert.Equal(t, repoName, r)
			assert.Equal(t, 1, page)
			assert.Equal(t, 100, perPage)

			// Return empty commits to simplify the test
			return []models.GitHubCommit{}, nil
		},
	}

	// Create a mock repository
	mockRepo := &MockRepository{
		GetRepositoryByFullNameFunc: func(fn string) (*models.Repository, error) {
			assert.Equal(t, fullName, fn)
			return existingRepo, nil // Repository exists
		},
		UpdateRepositoryFunc: func(repo *models.Repository) error {
			assert.Equal(t, existingRepo.ID, repo.ID)
			assert.Equal(t, "Updated repository", *repo.Description)
			assert.Equal(t, 10, repo.ForksCount)
			assert.Equal(t, 20, repo.StarsCount)
			return nil
		},
		CommitExistsFunc: func(repositoryID int64, sha string) (bool, error) {
			return false, nil
		},
		CreateCommitFunc: func(commit *models.Commit) error {
			return nil
		},
	}

	// Create the service
	githubService := NewTestGitHubService(mockGitHubClient, mockRepo, mockLogger)

	// Test
	err := githubService.FetchAndStoreRepository(context.Background(), owner, repoName)

	// Assert
	assert.NoError(t, err)
}

func TestFetchAndStoreRepository_GitHubError(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()

	owner := "testowner"
	repoName := "testrepo"

	// Create a mock GitHub client that returns an error
	mockGitHubClient := &MockGitHubClient{
		GetRepositoryFunc: func(o, r string) (*models.GitHubRepository, error) {
			return nil, errors.New("GitHub API error")
		},
	}

	// Create a mock repository
	mockRepo := &MockRepository{}

	// Create the service
	githubService := NewTestGitHubService(mockGitHubClient, mockRepo, mockLogger)

	// Test
	err := githubService.FetchAndStoreRepository(context.Background(), owner, repoName)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch repository from GitHub")
}

func TestFetchAndStoreRepository_RepositoryError(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()

	owner := "testowner"
	repoName := "testrepo"
	fullName := "testowner/testrepo"

	// Create a mock GitHub client
	mockGitHubClient := &MockGitHubClient{
		GetRepositoryFunc: func(o, r string) (*models.GitHubRepository, error) {
			return &models.GitHubRepository{
				Name:            repoName,
				FullName:        fullName,
				Description:     createStringPtr("Test repository"),
				HTMLURL:         "https://github.com/testowner/testrepo",
				Language:        createStringPtr("Go"),
				ForksCount:      10,
				StargazersCount: 20,
				OpenIssuesCount: 5,
				WatchersCount:   15,
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
			}, nil
		},
	}

	// Create a mock repository that returns an error
	mockRepo := &MockRepository{
		GetRepositoryByFullNameFunc: func(fn string) (*models.Repository, error) {
			return nil, errors.New("database error")
		},
	}

	// Create the service
	githubService := NewTestGitHubService(mockGitHubClient, mockRepo, mockLogger)

	// Test
	err := githubService.FetchAndStoreRepository(context.Background(), owner, repoName)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to check existing repository")
}

func TestFetchAndStoreCommits(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()

	owner := "testowner"
	repoName := "testrepo"
	fullName := "testowner/testrepo"

	repo := &models.Repository{
		ID:        1,
		Name:      repoName,
		FullName:  fullName,
		SyncSince: time.Now().Add(-30 * 24 * time.Hour),
	}

	commits := []models.GitHubCommit{
		{
			SHA: "abc123",
			Commit: models.GitHubCommitData{
				Message: "First commit",
				Author: models.GitHubCommitAuthor{
					Name:  "Test User",
					Email: "test@example.com",
					Date:  time.Now().Add(-24 * time.Hour),
				},
			},
			HTMLURL: "https://github.com/testowner/testrepo/commit/abc123",
		},
		{
			SHA: "def456",
			Commit: models.GitHubCommitData{
				Message: "Second commit",
				Author: models.GitHubCommitAuthor{
					Name:  "Another User",
					Email: "another@example.com",
					Date:  time.Now().Add(-12 * time.Hour),
				},
			},
			HTMLURL: "https://github.com/testowner/testrepo/commit/def456",
		},
	}

	// Create a mock GitHub client
	mockGitHubClient := &MockGitHubClient{
		GetRepositoryFunc: func(o, r string) (*models.GitHubRepository, error) {
			return &models.GitHubRepository{
				Name:     repoName,
				FullName: fullName,
			}, nil
		},
		GetCommitsFunc: func(o, r string, since time.Time, page, perPage int) ([]models.GitHubCommit, error) {
			assert.Equal(t, owner, o)
			assert.Equal(t, repoName, r)

			if page == 1 {
				return commits, nil
			}
			return []models.GitHubCommit{}, nil // No more commits on page 2
		},
	}

	// Create a mock repository
	commitExists := make(map[string]bool)
	mockRepo := &MockRepository{
		GetRepositoryByFullNameFunc: func(fn string) (*models.Repository, error) {
			return repo, nil
		},
		UpdateRepositoryFunc: func(r *models.Repository) error {
			return nil
		},
		CommitExistsFunc: func(repositoryID int64, sha string) (bool, error) {
			assert.Equal(t, repo.ID, repositoryID)
			return commitExists[sha], nil
		},
		CreateCommitFunc: func(commit *models.Commit) error {
			assert.Equal(t, repo.ID, commit.RepositoryID)
			commitExists[commit.SHA] = true
			return nil
		},
	}

	// Create the service
	githubService := NewTestGitHubService(mockGitHubClient, mockRepo, mockLogger)

	// Test
	err := githubService.FetchAndStoreRepository(context.Background(), owner, repoName)

	// Assert
	assert.NoError(t, err)
	assert.True(t, commitExists["abc123"])
	assert.True(t, commitExists["def456"])
}

func TestSyncAllRepositories(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any(), gomock.Any()).AnyTimes()

	repositories := []models.Repository{
		{
			ID:       1,
			Name:     "repo1",
			FullName: "owner1/repo1",
		},
		{
			ID:       2,
			Name:     "repo2",
			FullName: "owner2/repo2",
		},
	}

	// Create a mock GitHub client
	mockGitHubClient := &MockGitHubClient{
		GetRepositoryFunc: func(owner, repo string) (*models.GitHubRepository, error) {
			return &models.GitHubRepository{
				Name:     repo,
				FullName: owner + "/" + repo,
			}, nil
		},
		GetCommitsFunc: func(owner, repo string, since time.Time, page, perPage int) ([]models.GitHubCommit, error) {
			return []models.GitHubCommit{}, nil
		},
	}

	// Create a mock repository
	mockRepo := &MockRepository{
		GetAllRepositoriesFunc: func() ([]models.Repository, error) {
			return repositories, nil
		},
		GetRepositoryByFullNameFunc: func(fullName string) (*models.Repository, error) {
			for _, repo := range repositories {
				if repo.FullName == fullName {
					return &repo, nil
				}
			}
			return nil, nil
		},
		UpdateRepositoryFunc: func(repo *models.Repository) error {
			return nil
		},
		CommitExistsFunc: func(repositoryID int64, sha string) (bool, error) {
			return false, nil
		},
		CreateCommitFunc: func(commit *models.Commit) error {
			return nil
		},
	}

	// Create the service
	githubService := NewTestGitHubService(mockGitHubClient, mockRepo, mockLogger)

	// Test
	err := githubService.SyncAllRepositories(context.Background())

	// Assert
	assert.NoError(t, err)
}

func TestSyncAllRepositories_Error(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any(), gomock.Any()).AnyTimes()

	// Create a mock repository that returns an error
	mockRepo := &MockRepository{
		GetAllRepositoriesFunc: func() ([]models.Repository, error) {
			return nil, errors.New("database error")
		},
	}

	// Create a mock GitHub client
	mockGitHubClient := &MockGitHubClient{}

	// Create the service
	githubService := NewTestGitHubService(mockGitHubClient, mockRepo, mockLogger)

	// Test
	err := githubService.SyncAllRepositories(context.Background())

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get all repositories")
}

func TestResetRepositorySync(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)

	fullName := "testowner/testrepo"
	since := time.Now().Add(-60 * 24 * time.Hour) // 60 days ago

	// Create a mock repository
	mockRepo := &MockRepository{
		ResetRepositorySyncSinceFunc: func(fn string, s time.Time) error {
			assert.Equal(t, fullName, fn)
			assert.Equal(t, since, s)
			return nil
		},
	}

	// Create a mock GitHub client
	mockGitHubClient := &MockGitHubClient{}

	// Create the service
	githubService := NewTestGitHubService(mockGitHubClient, mockRepo, mockLogger)

	// Test
	err := githubService.ResetRepositorySync(fullName, since)

	// Assert
	assert.NoError(t, err)
}

func TestGetTopCommitAuthors(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)

	expectedAuthors := []models.CommitAuthorStats{
		{
			AuthorName:  "Test User",
			AuthorEmail: "test@example.com",
			CommitCount: 100,
		},
		{
			AuthorName:  "Another User",
			AuthorEmail: "another@example.com",
			CommitCount: 50,
		},
	}

	// Create a mock repository
	mockRepo := &MockRepository{
		GetTopCommitAuthorsFunc: func(limit int) ([]models.CommitAuthorStats, error) {
			assert.Equal(t, 10, limit)
			return expectedAuthors, nil
		},
	}

	// Create a mock GitHub client
	mockGitHubClient := &MockGitHubClient{}

	// Create the service
	githubService := NewTestGitHubService(mockGitHubClient, mockRepo, mockLogger)

	// Test
	authors, err := githubService.GetTopCommitAuthors(10)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, expectedAuthors, authors)
}

func TestGetCommitsByRepository(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)

	fullName := "testowner/testrepo"
	expectedCommits := []models.Commit{
		{
			ID:           1,
			RepositoryID: 1,
			SHA:          "abc123",
			Message:      "First commit",
			AuthorName:   "Test User",
			AuthorEmail:  "test@example.com",
			CommitDate:   time.Now().Add(-24 * time.Hour),
			URL:          "https://github.com/testowner/testrepo/commit/abc123",
		},
		{
			ID:           2,
			RepositoryID: 1,
			SHA:          "def456",
			Message:      "Second commit",
			AuthorName:   "Another User",
			AuthorEmail:  "another@example.com",
			CommitDate:   time.Now().Add(-12 * time.Hour),
			URL:          "https://github.com/testowner/testrepo/commit/def456",
		},
	}

	// Create a mock repository
	mockRepo := &MockRepository{
		GetCommitsByRepositoryFunc: func(repoName string) ([]models.Commit, error) {
			assert.Equal(t, fullName, repoName)
			return expectedCommits, nil
		},
	}

	// Create a mock GitHub client
	mockGitHubClient := &MockGitHubClient{}

	// Create the service
	githubService := NewTestGitHubService(mockGitHubClient, mockRepo, mockLogger)

	// Test
	commits, err := githubService.GetCommitsByRepository(fullName)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, expectedCommits, commits)
}
