package mocks

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github-service/internal/github"
	"github-service/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockGitHubClient is a mock implementation of the GitHubClient interface
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

func (m *MockGitHubClient) GetCommits(ctx context.Context, owner, repo string, since time.Time, page, perPage int) ([]*models.Commit, error) {
	args := m.Called(ctx, owner, repo, since, page, perPage)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Commit), args.Error(1)
}

type MockRepository struct {
	mock.Mock
}

// MockTransaction is a mock implementation of the Transaction interface
type MockTransaction struct {
	mock.Mock
}

func (m *MockRepository) GetRepository(ctx context.Context, owner, name string) (*models.Repository, error) {
	args := m.Called(ctx, owner, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Repository), args.Error(1)
}

// Add other required Repository interface methods...

func (m *MockTransaction) Commit() error {
	return m.Called().Error(0)
}

func (m *MockTransaction) Rollback() error {
	return m.Called().Error(0)
}

// MockRateLimitHandler is a mock implementation of the RateLimitHandler interface
type MockRateLimitHandler struct {
	mock.Mock
}


func (m *MockRateLimitHandler) RetryOperation(ctx context.Context, op func() error) error {
	args := m.Called(ctx, op)
	return args.Error(0)
}

// TestContext returns a context with timeout for testing
func TestContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

// TestRepository returns a test repository
func TestRepository() *models.Repository {
	now := time.Now()

	description := "A test repository"

	return &models.Repository{
		ID:          1,
		Name:        "test-repo",
		FullName:    "test-owner/test-repo",
		Description: &description,
		URL:         "https://github.com/test-owner/test-repo",
		CreatedAt:   now,
		UpdatedAt:   now,
		Language:    "Go",
	}
}

func TestClient_GetRepository(t *testing.T) {
	// Create a test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/test-owner/test-repo", r.URL.Path)
		assert.Equal(t, "token test-token", r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
            "id": 1,
            "name": "test-repo",
            "full_name": "test-owner/test-repo",
            "description": "A test repository",
            "private": false,
            "html_url": "https://github.com/test-owner/test-repo",
            "created_at": "2020-01-01T00:00:00Z",
            "updated_at": "2020-01-02T00:00:00Z",
            "pushed_at": "2020-01-03T00:00:00Z",
            "git_url": "git://github.com/test-owner/test-repo.git",
            "clone_url": "https://github.com/test-owner/test-repo.git",
            "language": "Go"
        }`))
	}))
	defer ts.Close()

	// Create client with test server URL
	client := github.NewClient("test-token")

	// Execute
	ctx := context.Background()
	repo, err := client.GetRepository(ctx, "test-owner", "test-repo")

	// Assert
	require.NoError(t, err)
	assert.Equal(t, int64(1), repo.ID)
	assert.Equal(t, "test-repo", repo.Name)
	assert.Equal(t, "test-owner/test-repo", repo.FullName)
	assert.Equal(t, "A test repository", repo.Description)
	assert.Equal(t, "Go", repo.Language)
}

func TestClient_HandleRateLimit(t *testing.T) {
	// Create a test server that returns rate limit error
	var callCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// First call: Return rate limit error
			w.Header().Set("X-RateLimit-Limit", "60")
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", "1234567890")
			w.Header().Set("X-RateLimit-Used", "60")
			w.Header().Set("X-RateLimit-Resource", "core")
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{
                "message": "API rate limit exceeded",
                "documentation_url": "https://docs.github.com/rest/overview/resources-in-the-rest-api#rate-limiting"
            }`))
			return
		}

		// Second call: Return success
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id": 1, "name": "test-repo"}`))
	}))
	defer ts.Close()

	// Create client with test server URL
	client := github.NewClient("test-token")

	// Execute
	ctx := context.Background()
	_, err := client.GetRepository(ctx, "test-owner", "test-repo")

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 2, callCount, "Expected client to retry after rate limit")
}
