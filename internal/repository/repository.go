package repository

import (
	"database/sql"
	"fmt"
	"time"

	"github-service/internal/models"
)

type Repository struct {
	db *sql.DB
}

func New(db *sql.DB) *Repository {
	return &Repository{db: db}
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

func (r *Repository) CreateRepository(repo *models.Repository) error {
	query := `
        INSERT INTO repositories (name, full_name, description, url, language, 
                                forks_count, stars_count, open_issues_count, 
                                watchers_count, created_at, updated_at, sync_since)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
        RETURNING id, created_at_db, updated_at_db`

	err := r.db.QueryRow(
		query, repo.Name, repo.FullName, repo.Description, repo.URL,
		repo.Language, repo.ForksCount, repo.StarsCount, repo.OpenIssuesCount,
		repo.WatchersCount, repo.CreatedAt, repo.UpdatedAt, repo.SyncSince,
	).Scan(&repo.ID, &repo.CreatedAtDB, &repo.UpdatedAtDB)

	if err != nil {
		return fmt.Errorf("failed to create repository: %w", err)
	}

	return nil
}

func (r *Repository) UpdateRepository(repo *models.Repository) error {
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

func (r *Repository) CommitExists(repositoryID int, sha string) (bool, error) {
	var count int
	query := `SELECT COUNT(*) FROM commits WHERE repository_id = $1 AND sha = $2`
	err := r.db.QueryRow(query, repositoryID, sha).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check if commit exists: %w", err)
	}
	return count > 0, nil
}

func (r *Repository) CreateCommit(commit *models.Commit) error {
	query := `
        INSERT INTO commits (repository_id, sha, message, author_name, 
                           author_email, commit_date, url)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
        ON CONFLICT (repository_id, sha) DO NOTHING
        RETURNING id, created_at`

	err := r.db.QueryRow(
		query, commit.RepositoryID, commit.SHA, commit.Message,
		commit.AuthorName, commit.AuthorEmail, commit.CommitDate, commit.URL,
	).Scan(&commit.ID, &commit.CreatedAt)

	if err != nil {
		// If no rows were returned due to conflict, that's okay
		if err == sql.ErrNoRows {
			return nil
		}
		return fmt.Errorf("failed to create commit: %w", err)
	}

	return nil
}

func (r *Repository) GetTopCommitAuthors(limit int) ([]models.CommitAuthorStats, error) {
	query := `
        SELECT author_name, author_email, COUNT(*) as commit_count
        FROM commits
        GROUP BY author_name
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
		if err := rows.Scan(&author.AuthorName, &author.CommitCount); err != nil {
			return nil, fmt.Errorf("failed to scan author stats: %w", err)
		}
		authors = append(authors, author)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate rows: %w", err)
	}

	return authors, nil
}

func (r *Repository) GetCommitsByRepository(repositoryName string) ([]models.Commit, error) {
	query := `
        SELECT c.id, c.repository_id, c.sha, c.message, c.author_name, 
               c.author_email, c.commit_date, c.url, c.created_at
        FROM commits c
        JOIN repositories r ON c.repository_id = r.id
        WHERE r.full_name = $1
        ORDER BY c.commit_date DESC`

	rows, err := r.db.Query(query, repositoryName)
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

func (r *Repository) GetAllRepositories() ([]models.Repository, error) {
	query := `
        SELECT id, name, full_name, description, url, language, forks_count, 
               stars_count, open_issues_count, watchers_count, created_at, 
               updated_at, last_synced_at, last_commit_sha, sync_since,
               created_at_db, updated_at_db
        FROM repositories 
        ORDER BY created_at_db`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all repositories: %w", err)
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
