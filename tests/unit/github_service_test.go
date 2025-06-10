package unit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github-service/internal/config"
	"github-service/internal/github"
	"github-service/internal/models"
	"github-service/internal/repository"
	"github-service/internal/service"
	"github-service/pkg/logger"
)

// MockGitHubClient is a mock implementation of the GitHub client
type MockGitHubClient struct {
	mock.Mock
}

func (m *MockGitHubClient) GetRepository(ctx context.Context, owner, repo string) (*models.Repository, error) {
	args := m.Called(ctx, owner, repo)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Repository), args.Error(1)
}


func (m *MockGitHubClient) GetCommits(ctx context.Context, owner, repo string, since *time.Time, page, perPage int) ([]*models.GitHubCommit, error) {
    args := m.Called(ctx, owner, repo, since, page, perPage)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).([]*models.GitHubCommit), args.Error(1)
}

func setupTestService(t *testing.T) (*service.GitHubService, *MockGitHubClient, sqlmock.Sqlmock, *repository.Repository, func()) {
	db, mockDB, err := sqlmock.New()
	require.NoError(t, err)

	log := logger.New("debug")
	repo := repository.New(db, log)
	txManager := repository.NewTransactionManager(db, log)

	// Create a test config
	testConfig := &config.Config{
		GitHubMaxRetries:     5,
		GitHubRetryMaxDelay:  1 * time.Second,
		GitHubRetryBaseDelay: 1 * time.Second,
	}

	rateLimitHandler := github.NewRateLimitHandler(testConfig, log)

	mockClient := new(MockGitHubClient)

	svc := service.NewGitHubService(
		mockClient,
		repo,
		log,
		rateLimitHandler,
		txManager,
	)

	return svc, mockClient, mockDB, repo, func() {
		db.Close()
	}
}

func TestGitHubService_FetchAndStoreRepository_Success(t *testing.T) {
	svc, mockClient, mockDB, _, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	owner := "test-owner"
	repoName := "test-repo"

	// Setup mock repository response
	mockRepo := &models.Repository{
		ID:              1,
		Name:            repoName,
		FullName:        owner + "/" + repoName,
		Description:     "Test repository",
		URL:             "https://github.com/test-owner/test-repo",
		Language:        "Go",
		ForksCount:      10,
		StarsCount:      20,
		OpenIssuesCount: 5,
		WatchersCount:   15,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	// Setup mock commits response
	mockCommits := []*models.Commit{
		{
			SHA:         "abc123",
			Message:     "Initial commit",
			AuthorName:  "test-author",
			AuthorEmail: "test@example.com",
			CommitDate:  time.Now(),
		},
	}

	// Setup expectations
	mockClient.On("GetRepository", ctx, owner, repoName).Return(mockRepo, nil)
	mockClient.On("ListCommits", ctx, owner, repoName, mock.Anything, 1, 100).Return(mockCommits, nil)

	// Setup database expectations
	mockDB.ExpectBegin()
	mockDB.ExpectQuery("INSERT INTO repositories").
		WithArgs(mockRepo.Name, mockRepo.FullName, mockRepo.Description, mockRepo.URL,
			mockRepo.Language, mockRepo.ForksCount, mockRepo.StarsCount, mockRepo.OpenIssuesCount,
			mockRepo.WatchersCount, mockRepo.CreatedAt, mockRepo.UpdatedAt, mock.Anything).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	mockDB.ExpectQuery("INSERT INTO commits").
		WithArgs(1, mockCommits[0].SHA, mockCommits[0].Message, mockCommits[0].AuthorName,
			mockCommits[0].AuthorEmail, mockCommits[0].CommitDate,
			mock.Anything, mock.Anything).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	mockDB.ExpectCommit()

	// Execute
	err := svc.TxManager.WithTransaction(ctx, "test_fetch_store", func(tx repository.Transaction) error {
		return svc.FetchAndStoreRepository(ctx, tx, owner, repoName)
	})

	// Verify
	assert.NoError(t, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
	mockClient.AssertExpectations(t)
}

func TestGitHubService_FetchAndStoreRepository_ErrorGettingRepo(t *testing.T) {
	svc, mockClient, _, _, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	owner := "test-owner"
	repoName := "test-repo"

	// Setup mock to return error
	expectedErr := errors.New("failed to get repository")
	mockClient.On("GetRepository", ctx, owner, repoName).Return((*models.Repository)(nil), expectedErr)

	// Execute
	err := svc.TxManager.WithTransaction(ctx, "test_error", func(tx repository.Transaction) error {
		return svc.FetchAndStoreRepository(ctx, tx, owner, repoName)
	})

	// Verify
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get repository")
	mockClient.AssertExpectations(t)
}

func TestGitHubService_FetchAndStoreRepository_ErrorListingCommits(t *testing.T) {
	svc, mockClient, mockDB, _, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	owner := "test-owner"
	repoName := "test-repo"

	// Setup mock repository
	mockRepo := &models.Repository{
		ID:       1,
		Name:     repoName,
		FullName: owner + "/" + repoName,
	}

	// Setup mock to return error for commits
	expectedErr := errors.New("failed to list commits")
	mockClient.On("GetRepository", ctx, owner, repoName).Return(mockRepo, nil)
	mockClient.On("ListCommits", ctx, owner, repoName, mock.Anything, 1, 100).Return(([]*models.Commit)(nil), expectedErr)

	// Setup database expectations for rollback
	mockDB.ExpectBegin()
	mockDB.ExpectRollback()

	// Execute
	err := svc.TxManager.WithTransaction(ctx, "test_commit_error", func(tx repository.Transaction) error {
		return svc.FetchAndStoreRepository(ctx, tx, owner, repoName)
	})

	// Verify
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list commits")
	assert.NoError(t, mockDB.ExpectationsWereMet())
	mockClient.AssertExpectations(t)
}

func TestGitHubService_FetchAndStoreRepository_RateLimitHandling(t *testing.T) {
	svc, mockClient, mockDB, _, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	owner := "test-owner"
	repoName := "test-repo"

	// Setup mock repository
	mockRepo := &models.Repository{
		ID:       1,
		Name:     repoName,
		FullName: owner + "/" + repoName,
	}

	// Setup rate limit error
	rateLimitErr := &models.RateLimitError{
		Response: &http.Response{
			StatusCode: http.StatusForbidden,
			Header: http.Header{
				"X-Ratelimit-Reset": []string{fmt.Sprintf("%d", time.Now().Add(time.Hour).Unix())},
			},
		},
		Message: "API rate limit exceeded",
	}

	mockClient.On("GetRepository", ctx, owner, repoName).Return(mockRepo, nil)
	mockClient.On("ListCommits", ctx, owner, repoName, mock.Anything, 1, 100).Return(([]*models.Commit)(nil), rateLimitErr)

	// Setup database expectations for rollback
	mockDB.ExpectBegin()
	mockDB.ExpectRollback()

	// Execute
	start := time.Now()
	err := svc.TxManager.WithTransaction(ctx, "test_rate_limit", func(tx repository.Transaction) error {
		return svc.FetchAndStoreRepository(ctx, tx, owner, repoName)
	})
	duration := time.Since(start)

	// Verify
	assert.Error(t, err)
	assert.True(t, duration >= time.Second, "should have waited for rate limit")
	assert.Contains(t, err.Error(), "rate limit exceeded")
	assert.NoError(t, mockDB.ExpectationsWereMet())
	mockClient.AssertExpectations(t)
}

func TestGitHubService_SyncAllRepositories(t *testing.T) {
	svc, mockClient, mockDB, _, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()

	// Setup test repositories in the database
	repos := []*models.Repository{
		{ID: 1, FullName: "owner1/repo1"},
		{ID: 2, FullName: "owner2/repo2"},
	}

	// Setup mock repository responses
	for _, repo := range repos {
		owner, name := splitFullName(repo.FullName)
		mockRepo := &models.Repository{
			ID:       int64(repo.ID),
			Name:     name,
			FullName: repo.FullName,
		}
		mockClient.On("GetRepository", ctx, owner, name).Return(mockRepo, nil)
		mockClient.On("ListCommits", ctx, owner, name, mock.Anything, 1, 100).Return([]*models.Commit{}, nil)
	}

	// Setup database expectations
	mockDB.ExpectQuery("SELECT id, full_name FROM repositories").WillReturnRows(
		sqlmock.NewRows([]string{"id", "full_name"}).
			AddRow(repos[0].ID, repos[0].FullName).
			AddRow(repos[1].ID, repos[1].FullName),
	)

	for range repos {
		mockDB.ExpectBegin()
		mockDB.ExpectQuery("UPDATE repositories").
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
		mockDB.ExpectCommit()
	}

	// Execute
	err := svc.TxManager.WithTransaction(ctx, "test_sync_all_repositories", func(tx repository.Transaction) error {
		return svc.SyncAllRepositories(ctx, tx)
	})	

	// Verify
	assert.NoError(t, err)
	assert.NoError(t, mockDB.ExpectationsWereMet())
	mockClient.AssertExpectations(t)
}

// Helper function to split full name into owner and repo
func splitFullName(fullName string) (string, string) {
	parts := strings.Split(fullName, "/")
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}
