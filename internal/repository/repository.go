package repository

import (
	"database/sql"
	"fmt"
	"time"

	"github-service/internal/models"
	"github-service/pkg/logger"

	"github.com/pkg/errors"
)

type Repository struct {
	db     *sql.DB
	logger logger.Logger
}

func New(db *sql.DB, logger logger.Logger) *Repository {
	return &Repository{db: db, logger: logger}
}

func (r *Repository) UpdateRepositorySyncStatus(tx Transaction, id int64, status models.SyncStatus, syncedAt time.Time) error {
	query := `
		UPDATE repositories 
		SET sync_status = $1, last_synced_at = $2, updated_at = $3
		WHERE id = $4`

	result, err := tx.Exec(query, status, syncedAt, time.Now(), id)
	if err != nil {
		return errors.Wrap(err, "failed to update repository sync status")
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "failed to get rows affected")
	}

	if rowsAffected == 0 {
		return errors.New("repository not found")
	}

	r.logger.Info("repository sync status updated",
		"repository_id", id,
		"status", status,
		"synced_at", syncedAt)

	return nil
}

func (r *Repository) GetRepositoryForUpdate(tx Transaction, owner, name string) (*models.Repository, error) {
	query := `
		SELECT id, owner, name, full_name, description, default_branch, 
			   language, star_count, fork_count, issue_count, 
			   last_synced_at, sync_since, created_at, updated_at
		FROM repositories 
		WHERE owner = $1 AND name = $2
		FOR UPDATE`

	var repo models.Repository
	err := tx.QueryRow(query, owner, name).Scan(
		&repo.ID, &repo.Name, &repo.FullName,
		&repo.Description, &repo.Language,
		&repo.StarsCount, &repo.ForksCount, &repo.OpenIssuesCount,
		&repo.LastSyncedAt, &repo.SyncSince,
		&repo.CreatedAt, &repo.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("repository not found")
		}
		return nil, errors.Wrap(err, "failed to get repository for update")
	}

	return &repo, nil
}

func (r *Repository) GetRepositoryByFullName(fullName string) (*models.Repository, error) {
	query := `
        SELECT id, name, full_name, description, url, language, forks_count, 
               stars_count, open_issues_count, watchers_count, created_at, 
               updated_at, last_synced_at, last_commit_sha, sync_since,
               created_at_db, updated_at_db
        FROM repositories 
        WHERE full_name = $1`

	var repo models.Repository
	err := r.db.QueryRow(query, fullName).Scan(
		&repo.ID, &repo.Name, &repo.FullName, &repo.Description,
		&repo.URL, &repo.Language, &repo.ForksCount, &repo.StarsCount,
		&repo.OpenIssuesCount, &repo.WatchersCount, &repo.CreatedAt,
		&repo.UpdatedAt, &repo.LastSyncedAt, &repo.LastCommitSHA,
		&repo.SyncSince, &repo.CreatedAtDB, &repo.UpdatedAtDB,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get repository: %w", err)
	}

	return &repo, nil
}

func (r *Repository) CreateRepository(tx Transaction, repo *models.Repository) error {
	query := `
        INSERT INTO repositories (name, full_name, description, url, language, 
                                forks_count, stars_count, open_issues_count, 
                                watchers_count, created_at, updated_at, sync_since)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
        RETURNING id, created_at_db, updated_at_db`

	err := tx.QueryRow(
		query, repo.Name, repo.FullName, repo.Description, repo.URL,
		repo.Language, repo.ForksCount, repo.StarsCount, repo.OpenIssuesCount,
		repo.WatchersCount, repo.CreatedAt, repo.UpdatedAt, repo.SyncSince,
	).Scan(&repo.ID, &repo.CreatedAtDB, &repo.UpdatedAtDB)

	if err != nil {
		return errors.Wrap(err, "failed to create repository")
	}

	r.logger.Info("repository created",
		"repository_id", repo.ID,
		"full_name", repo.FullName)

	return nil
}

func (r *Repository) UpdateRepository(tx Transaction, repo *models.Repository) error {
	query := `
        UPDATE repositories 
        SET name = $2, description = $3, url = $4, language = $5, 
            forks_count = $6, stars_count = $7, open_issues_count = $8, 
            watchers_count = $9, updated_at = $10, last_synced_at = $11,
            last_commit_sha = $12, updated_at_db = NOW()
        WHERE id = $1`

	_, err := r.db.Exec(
		query, repo.ID, repo.Name, repo.Description, repo.URL,
		repo.Language, repo.ForksCount, repo.StarsCount, repo.OpenIssuesCount,
		repo.WatchersCount, repo.UpdatedAt, repo.LastSyncedAt, repo.LastCommitSHA,
	)

	if err != nil {
		return fmt.Errorf("failed to update repository: %w", err)
	}

	r.logger.Info("repository updated",
		"repository_id", repo.ID,
		"full_name", repo.FullName)

	return nil
}

func (r *Repository) ResetRepositorySyncSince(fullName string, since time.Time) error {
	query := `UPDATE repositories SET sync_since = $1, last_commit_sha = NULL WHERE full_name = $2`
	_, err := r.db.Exec(query, since, fullName)
	if err != nil {
		return fmt.Errorf("failed to reset repository sync since: %w", err)
	}

	// Delete all commits after the since date
	deleteQuery := `
        DELETE FROM commits 
        WHERE repository_id = (SELECT id FROM repositories WHERE full_name = $1)
        AND commit_date >= $2`
	_, err = r.db.Exec(deleteQuery, fullName, since)
	if err != nil {
		return fmt.Errorf("failed to delete commits after reset date: %w", err)
	}

	return nil
}

func (r *Repository) CommitExists(tx Transaction, repositoryID int64, sha string) (bool, error) {
	var count int
	err := tx.QueryRow(
		"SELECT COUNT(*) FROM commits WHERE repository_id = $1 AND sha = $2",
		repositoryID, sha,
	).Scan(&count)

	if err != nil {
		return false, fmt.Errorf("failed to check if commit exists: %w", err)
	}

	return count > 0, nil
}

func (r *Repository) CreateCommit(tx Transaction, commit *models.Commit) error {
	query := `
		INSERT INTO commits (repository_id, sha, message, author_name, author_email, commit_date, url, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at`

	err := tx.QueryRow(
		query,
		commit.RepositoryID,
		commit.SHA,
		commit.Message,
		commit.AuthorName,
		commit.AuthorEmail,
		commit.CommitDate,
		commit.URL,
		time.Now(),
		time.Now(),
	).Scan(&commit.ID, &commit.CreatedAt, &commit.UpdatedAt)

	if err != nil {
		return errors.Wrap(err, "failed to create commit")
	}

	return nil
}

func (r *Repository) GetTopCommitAuthors(limit int) ([]models.CommitAuthorStats, error) {
	query := `
        SELECT author_name, author_email, COUNT(*) as commit_count
        FROM commits
        GROUP BY author_name, author_email
        ORDER BY commit_count DESC
        LIMIT $1`

	rows, err := r.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get top commit authors: %w", err)
	}
	defer rows.Close()

	var authors []models.CommitAuthorStats
	for rows.Next() {
		var author models.CommitAuthorStats
		if err := rows.Scan(&author.AuthorName, &author.AuthorEmail, &author.CommitCount); err != nil {
			return nil, fmt.Errorf("failed to scan author stats: %w", err)
		}
		authors = append(authors, author)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate rows: %w", err)
	}

	return authors, nil
}

// GetCommitsByRepository retrieves commits for a specific repository with pagination
// page starts from 1, pageSize is the number of items per page
func (r *Repository) GetCommitsByRepository(repositoryName string, page, pageSize int) ([]models.Commit, error) {
	// Input validation
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10 // Default page size
	}
	offset := (page - 1) * pageSize

	// Optimized query with pagination and efficient JOIN
	// Using a subquery to first get the repository ID, then joining with commits
	query := `
        WITH repo AS (
            SELECT id FROM repositories WHERE full_name = $1 LIMIT 1
        )
        SELECT c.id, c.repository_id, c.sha, c.message, c.author_name, 
               c.author_email, c.commit_date, c.url, c.created_at
        FROM commits c
        WHERE c.repository_id = (SELECT id FROM repo)
        ORDER BY c.commit_date DESC
        LIMIT $2 OFFSET $3`

	rows, err := r.db.Query(query, repositoryName, pageSize, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get commits by repository: %w", err)
	}
	defer rows.Close()

	var commits []models.Commit
	for rows.Next() {
		var commit models.Commit
		if err := rows.Scan(
			&commit.ID, &commit.RepositoryID, &commit.SHA, &commit.Message,
			&commit.AuthorName, &commit.AuthorEmail, &commit.CommitDate,
			&commit.URL, &commit.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan commit: %w", err)
		}
		commits = append(commits, commit)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate rows: %w", err)
	}

	return commits, nil
}

// GetAllRepositories retrieves a paginated list of all repositories
// page starts from 1, pageSize is the number of items per page (max 100)
func (r *Repository) GetAllRepositories(page, pageSize int) ([]models.Repository, error) {
	// Input validation
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50 // Default page size
	}
	offset := (page - 1) * pageSize

	// Only select necessary columns and use parameterized query for pagination
	query := `
        SELECT id, name, full_name, description, url, language, 
               forks_count, stars_count, open_issues_count, watchers_count, 
               created_at, updated_at, last_synced_at, last_commit_sha, 
               sync_since, created_at_db, updated_at_db
        FROM repositories 
        ORDER BY created_at_db DESC
        LIMIT $1 OFFSET $2`

	rows, err := r.db.Query(query, pageSize, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get repositories: %w", err)
	}
	defer rows.Close()

	var repositories []models.Repository
	for rows.Next() {
		var repo models.Repository
		if err := rows.Scan(
			&repo.ID, &repo.Name, &repo.FullName, &repo.Description,
			&repo.URL, &repo.Language, &repo.ForksCount, &repo.StarsCount,
			&repo.OpenIssuesCount, &repo.WatchersCount, &repo.CreatedAt,
			&repo.UpdatedAt, &repo.LastSyncedAt, &repo.LastCommitSHA,
			&repo.SyncSince, &repo.CreatedAtDB, &repo.UpdatedAtDB,
		); err != nil {
			return nil, fmt.Errorf("failed to scan repository: %w", err)
		}
		repositories = append(repositories, repo)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate rows: %w", err)
	}

	return repositories, nil
}

func (r *Repository) GetCommitCountByRepository(repositoryID int64) ([]models.CommitCountResponse, error) {
	query := `
        SELECT COUNT(*) as commit_count
        FROM commits
        where repository_id = $1`

	rows, err := r.db.Query(query, repositoryID)
	if err != nil {
		return nil, fmt.Errorf("failed to get commits count by repository: %w", err)
	}
	defer rows.Close()

	var commits []models.CommitCountResponse
	for rows.Next() {
		var commit models.CommitCountResponse
		if err := rows.Scan(&commit.CommitCount); err != nil {
			return nil, fmt.Errorf("failed to scan commit count: %w", err)
		}
		commits = append(commits, commit)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate rows: %w", err)
	}

	return commits, nil
}
