package unit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github-service/internal/github"
	"github-service/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_GetRepository(t *testing.T) {
	t.Parallel()

	var description string = "Test repository"

	testRepo := &models.Repository{
		ID:          1,
		Name:        "test-repo",
		FullName:    "test-owner/test-repo",
		Description: &description,
		URL:         "https://github.com/test-owner/test-repo",
		CreatedAt:   time.Now().Add(-24 * time.Hour),
		UpdatedAt:   time.Now(),
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/test-owner/test-repo", r.URL.Path)
		assert.Equal(t, "token test-token", r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(testRepo)
	}))
	defer ts.Close()

	client := github.NewClient("test-token")
	client.SetBaseURL(ts.URL)
	repo, err := client.GetRepository(context.Background(), "test-owner", "test-repo")

	require.NoError(t, err)
	assert.Equal(t, testRepo.FullName, repo.FullName)
	assert.Equal(t, testRepo.Description, repo.Description)
}

func TestClient_GetRepository_NotFound(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message":           "Not Found",
			"documentation_url": "https://docs.github.com/rest/repos/repos#get-a-repository",
		})
	}))
	defer ts.Close()

	client := github.NewClient("test-token")
	client.SetBaseURL(ts.URL)
	_, err := client.GetRepository(context.Background(), "test-owner", "nonexistent-repo")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Not Found")
}

func TestClient_GetCommits(t *testing.T) {
	t.Parallel()

	testCommits := []*models.GitHubCommit{
		{
			SHA: "abc123",
			Commit: models.GitHubCommitData{
				Message: "Initial commit",
				Author: models.GitHubCommitAuthor{
					Name:  "Test User",
					Email: "test@example.com",
					Date:  time.Now(),
				},
			},
			HTMLURL: "https://api.github.com/repos/test-owner/test-repo/commit/abc123",
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/test-owner/test-repo/commits", r.URL.Path)
		assert.Equal(t, "token test-token", r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(testCommits)
	}))
	defer ts.Close()

	client := github.NewClient("test-token")
	client.SetBaseURL(ts.URL)
	commits, err := client.GetCommits(context.Background(), "test-owner", "test-repo", &time.Time{}, 1, 30)

	require.NoError(t, err)
	require.Len(t, commits, 1)
	assert.Equal(t, testCommits[0].SHA, commits[0].SHA)
	assert.Equal(t, testCommits[0].Commit.Message, commits[0].Commit.Message)
}
