package unit

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github-service/internal/models"
	"github-service/internal/repository"
	"github-service/pkg/logger"
)

func TestRepository_GetRepositoryByFullName(t *testing.T) {
	db, mock, err := sqlmock.New()
	logger := logger.New("Info")
	require.NoError(t, err)
	defer db.Close()

	repo := repository.New(db, logger)

	t.Run("repository exists", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{
			"id", "name", "full_name", "description", "url", "language",
			"forks_count", "stars_count", "open_issues_count", "watchers_count",
			"created_at", "updated_at", "last_synced_at", "last_commit_sha",
			"sync_since", "created_at_db", "updated_at_db",
		}).AddRow(
			1, "test-repo", "owner/test-repo", "Test repository", "https://github.com/owner/test-repo",
			"Go", 10, 20, 5, 15, time.Now(), time.Now(), nil, nil,
			time.Now(), time.Now(), time.Now(),
		)

		mock.ExpectQuery("SELECT (.+) FROM repositories WHERE full_name = \\$1").
			WithArgs("owner/test-repo").
			WillReturnRows(rows)

		result, err := repo.GetRepositoryByFullName("owner/test-repo")
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "test-repo", result.Name)
		assert.Equal(t, "owner/test-repo", result.FullName)
	})

	t.Run("repository not found", func(t *testing.T) {
		mock.ExpectQuery("SELECT (.+) FROM repositories WHERE full_name = \\$1").
			WithArgs("owner/nonexistent").
			WillReturnError(sql.ErrNoRows)

		result, err := repo.GetRepositoryByFullName("owner/nonexistent")
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRepository_CreateRepository(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	logger := logger.New("debug")
	repo := repository.New(db, logger)
	txManager := repository.NewTransactionManager(db, logger)

	testRepo := &models.Repository{
		Name:            "test-repo",
		FullName:        "owner/test-repo",
		Description:     stringPtr("Test repository"),
		URL:             "https://github.com/owner/test-repo",
		Language:        stringPtr("Go"),
		ForksCount:      10,
		StarsCount:      20,
		OpenIssuesCount: 5,
		WatchersCount:   15,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		SyncSince:       time.Now(),
	}

	// Set up the mock expectations in the correct order
	mock.ExpectBegin() // This should come before BeginTransaction
	rows := sqlmock.NewRows([]string{"id", "created_at_db", "updated_at_db"}).
		AddRow(1, time.Now(), time.Now())
	mock.ExpectQuery("INSERT INTO repositories").
		WithArgs(testRepo.Name, testRepo.FullName, testRepo.Description, testRepo.URL,
			testRepo.Language, testRepo.ForksCount, testRepo.StarsCount, testRepo.OpenIssuesCount,
			testRepo.WatchersCount, testRepo.CreatedAt, testRepo.UpdatedAt, testRepo.SyncSince).
		WillReturnRows(rows)

	// Start the transaction after setting up the expectations
	ctx := context.Background()
	tx, err := txManager.BeginTransaction(ctx, "test")
	require.NoError(t, err)
	defer tx.Rollback()

	// Now execute the test
	err = repo.CreateRepository(tx, testRepo)
	require.NoError(t, err)
	assert.Equal(t, int64(1), testRepo.ID) // Changed from 1 to int64(1) to match the type

	// Verify all expectations were met
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRepository_CommitExists(t *testing.T) {
	db, mock, err := sqlmock.New()
	logger := logger.New("Info")
	require.NoError(t, err)
	defer db.Close()

	repo := repository.New(db, logger)

	t.Run("commit exists", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"count"}).AddRow(1)
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM commits WHERE repository_id = \\$1 AND sha = \\$2").
			WithArgs(1, "abc123").
			WillReturnRows(rows)

		exists, err := repo.CommitExists(1, "abc123")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("commit does not exist", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"count"}).AddRow(0)
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM commits WHERE repository_id = \\$1 AND sha = \\$2").
			WithArgs(1, "def456").
			WillReturnRows(rows)

		exists, err := repo.CommitExists(1, "def456")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRepository_GetTopCommitAuthors(t *testing.T) {
	db, mock, err := sqlmock.New()
	logger := logger.New("Info")
	require.NoError(t, err)
	defer db.Close()

	repo := repository.New(db, logger)

	rows := sqlmock.NewRows([]string{"author_name", "author_email", "commit_count"}).
		AddRow("John Doe", "john.doe@example.com", 100).
		AddRow("Jane Smith", "jane.smith@example.com", 75).
		AddRow("Bob Wilson", "bob.wilson@example.com", 50)

	mock.ExpectQuery("SELECT author_name, author_email, COUNT\\(\\*\\) as commit_count FROM commits GROUP BY author_name, author_email ORDER BY commit_count DESC LIMIT \\$1").
		WithArgs(3).
		WillReturnRows(rows)

	authors, err := repo.GetTopCommitAuthors(3)
	require.NoError(t, err)
	assert.Len(t, authors, 3)
	assert.Equal(t, "John Doe", authors[0].AuthorName)
	assert.Equal(t, "john.doe@example.com", authors[0].AuthorEmail)
	assert.Equal(t, 100, authors[0].CommitCount)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func stringPtr(s string) *string {
	return &s
}
