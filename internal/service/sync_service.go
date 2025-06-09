package service

// import (
// 	"context"
// 	"fmt"
// 	"log/slog"
// 	"strings"
// 	"time"

// 	"github-service/internal/database"
// 	"github-service/internal/github"
// 	"github-service/internal/models"
// 	"github-service/internal/repository"

// 	"github.com/pkg/errors"
// )

// // SyncService handles repository synchronization with transactional consistency
// type SyncService struct {
// 	txManager    repository.TransactionManager
// 	repoRepo     repository.Repository
// 	githubClient github.Client
// 	logger       *slog.Logger
// 	batchSize    int
// 	maxRetries   int
// 	retryDelay   time.Duration
// }

// // NewSyncService creates a new sync service with dependencies
// func NewSyncService(
// 	txManager repository.TransactionManager,
// 	repoRepo repository.Repository,
// 	githubClient github.Client,
// 	logger *slog.Logger,
// ) *SyncService {
// 	return &SyncService{
// 		txManager:    txManager,
// 		repoRepo:     repoRepo,
// 		githubClient: githubClient,
// 		logger:       logger,
// 		batchSize:    100,
// 		maxRetries:   3,
// 		retryDelay:   time.Second * 2,
// 	}
// }

// // SyncRepositoryRequest represents a repository sync request
// type SyncRepositoryRequest struct {
// 	Owner      string     `json:"owner" validate:"required,min=1,max=50"`
// 	Repository string     `json:"repository" validate:"required,min=1,max=100"`
// 	SyncSince  *time.Time `json:"sync_since,omitempty"`
// 	Force      bool       `json:"force,omitempty"`
// }

// // SyncRepositoryResponse represents the sync operation result
// type SyncRepositoryResponse struct {
// 	Repository     *repository.Repository `json:"repository"`
// 	CommitsAdded   int                    `json:"commits_added"`
// 	CommitsUpdated int                    `json:"commits_updated"`
// 	CommitsDeleted int                    `json:"commits_deleted"`
// 	SyncDuration   time.Duration          `json:"sync_duration"`
// 	SyncedAt       time.Time              `json:"synced_at"`
// }

// // FullRepositorySync performs a complete repository synchronization within a single transaction
// func (s *SyncService) FullRepositorySync(ctx context.Context, req SyncRepositoryRequest) (*SyncRepositoryResponse, error) {
// 	startTime := time.Now()

// 	s.logger.Info("starting full repository sync",
// 		"owner", req.Owner,
// 		"repository", req.Repository,
// 		"sync_since", req.SyncSince,
// 		"force", req.Force)

// 	var response *SyncRepositoryResponse

// 	// Execute the entire sync operation within a single transaction
// 	err := s.txManager.WithTransactionTimeout(ctx, "full_repository_sync", 10*time.Minute, func(tx database.Transaction) error {
// 		var err error
// 		response, err = s.performFullSync(tx, req, startTime)
// 		return err
// 	})

// 	if err != nil {
// 		s.logger.Error("full repository sync failed",
// 			"owner", req.Owner,
// 			"repository", req.Repository,
// 			"duration", time.Since(startTime),
// 			"error", err)
// 		return nil, errors.Wrap(err, "full repository sync failed")
// 	}

// 	s.logger.Info("full repository sync completed successfully",
// 		"owner", req.Owner,
// 		"repository", req.Repository,
// 		"commits_added", response.CommitsAdded,
// 		"commits_updated", response.CommitsUpdated,
// 		"commits_deleted", response.CommitsDeleted,
// 		"duration", response.SyncDuration)

// 	return response, nil
// }

// // performFullSync executes the sync logic within a transaction
// func (s *SyncService) performFullSync(tx repository.Transaction, req SyncRepositoryRequest, startTime time.Time) (*SyncRepositoryResponse, error) {
// 	// Step 1: Fetch or create repository with row lock
// 	repo, err := s.getOrCreateRepository(tx, req.Owner, req.Repository)
// 	if err != nil {
// 		return nil, errors.Wrap(err, "failed to get or create repository")
// 	}

// 	// Step 2: Check if sync is needed (unless forced)
// 	if !req.Force && !s.shouldSync(repo) {
// 		s.logger.Info("repository sync skipped - not needed",
// 			"repository_id", repo.ID,
// 			"last_synced", repo.LastSyncedAt,
// 			"sync_status", repo.SyncStatus)

// 		return &SyncRepositoryResponse{
// 			Repository:   repo,
// 			SyncDuration: time.Since(startTime),
// 			SyncedAt:     time.Now(),
// 		}, nil
// 	}

// 	// Step 3: Update sync status to in_progress
// 	if err := s.repoRepo.UpdateRepositorySyncStatus(tx, repo.ID, models.SyncStatusInProgress, time.Now()); err != nil {
// 		return nil, errors.Wrap(err, "failed to update sync status to in_progress")
// 	}

// 	// Step 4: Fetch repository metadata from GitHub and update
// 	updatedRepo, err := s.updateRepositoryMetadata(tx, repo)
// 	if err != nil {
// 		// Update status to failed before returning
// 		s.repoRepo.UpdateRepositorySyncStatus(tx, repo.ID, models.SyncStatusFailed, time.Now())
// 		return nil, errors.Wrap(err, "failed to update repository metadata")
// 	}

// 	// Step 5: Determine sync date
// 	syncSince := s.determineSyncSince(updatedRepo, req.SyncSince)

// 	// Step 6: Handle reset sync if requested
// 	var commitsDeleted int
// 	if req.SyncSince != nil {
// 		deleted, err := s.resetCommitsSince(tx, repo.ID, *req.SyncSince)
// 		if err != nil {
// 			s.repoRepo.UpdateRepositorySyncStatus(tx, repo.ID, models.SyncStatusFailed, time.Now())
// 			return nil, errors.Wrap(err, "failed to reset commits")
// 		}
// 		commitsDeleted = deleted
// 	}

// 	// Step 7: Fetch and sync commits
// 	commitsAdded, commitsUpdated, err := s.syncCommits(tx, repo.ID, req.Owner, req.Repository, syncSince)
// 	if err != nil {
// 		s.repoRepo.UpdateRepositorySyncStatus(tx, repo.ID, models.SyncStatusFailed, time.Now())
// 		return nil, errors.Wrap(err, "failed to sync commits")
// 	}

// 	// Step 8: Update commit statistics
// 	if err := s.updateCommitStatistics(tx, repo.ID); err != nil {
// 		s.logger.Warn("failed to update commit statistics", "error", err)
// 		// Don't fail the transaction for statistics update
// 	}

// 	// Step 9: Update repository sync status to completed
// 	syncedAt := time.Now()
// 	if err := s.repoRepo.UpdateRepositorySyncStatus(tx, repo.ID, models.SyncStatusCompleted, syncedAt); err != nil {
// 		return nil, errors.Wrap(err, "failed to update sync status to completed")
// 	}

// 	// Step 10: Update sync_since timestamp
// 	updatedRepo.SyncSince = &syncedAt
// 	updatedRepo.LastSyncedAt = &syncedAt

// 	return &SyncRepositoryResponse{
// 		Repository:     updatedRepo,
// 		CommitsAdded:   commitsAdded,
// 		CommitsUpdated: commitsUpdated,
// 		CommitsDeleted: commitsDeleted,
// 		SyncDuration:   time.Since(startTime),
// 		SyncedAt:       syncedAt,
// 	}, nil
// }

// // getOrCreateRepository retrieves existing repository or creates a new one
// func (s *SyncService) getOrCreateRepository(tx repository.Transaction, owner, name string) (*models.Repository, error) {
// 	// Try to get existing repository with row lock
// 	repo, err := s.repoRepo.GetRepositoryForUpdate(tx, owner, name)
// 	if err != nil && err.Error() != "repository not found" {
// 		return nil, errors.Wrap(err, "failed to get repository")
// 	}

// 	// If repository doesn't exist, create it
// 	if repo == nil {
// 		// Fetch repository metadata from GitHub

// 		response, err := s.githubClient.GetRepository(tx.Context(), owner, name)

// 		if err != nil {
// 			return nil, errors.Wrap(err, "failed to fetch repository from GitHub")
// 		}

// 		githubRepo, err := s.githubClient.ParseRepositoryResponse(response)

// 		repo = &models.Repository{
// 			Name:            name,
// 			FullName:        fmt.Sprintf("%s/%s", owner, name),
// 			Description:     githubRepo.Description,
// 			Language:        githubRepo.Language,
// 			StarsCount:      githubRepo.StarsCount,
// 			ForksCount:      githubRepo.ForksCount,
// 			OpenIssuesCount: githubRepo.OpenIssuesCount,
// 			SyncStatus:      models.SyncStatusPending,
// 		}

// 		if err := s.repoRepo.CreateRepository(tx, repo); err != nil {
// 			return nil, errors.Wrap(err, "failed to create repository")
// 		}

// 		s.logger.Info("repository created",
// 			"repository_id", repo.ID,
// 			"full_name", repo.FullName)
// 	}

// 	return repo, nil
// }

// // updateRepositoryMetadata fetches and updates repository metadata from GitHub
// func (s *SyncService) updateRepositoryMetadata(tx repository.Transaction, repo *models.Repository) (*models.Repository, error) {
// 	response, err := s.githubClient.GetRepository(tx.Context(), strings.Split(repo.FullName, "/")[0], repo.Name)
// 	if err != nil {
// 		return nil, errors.Wrap(err, "failed to fetch repository metadata from GitHub")
// 	}

// 	githubRepo, err := s.githubClient.ParseRepositoryResponse(response)
// 	if err != nil {
// 		return nil, errors.Wrap(err, "failed to parse repository response")
// 	}

// 	// Update repository metadata
// 	repo.Description = githubRepo.Description
// 	repo.Language = githubRepo.Language
// 	repo.StarsCount = githubRepo.StarsCount
// 	repo.ForksCount = githubRepo.ForksCount
// 	repo.OpenIssuesCount = githubRepo.OpenIssuesCount

// 	if err := s.repoRepo.UpdateRepository(tx, repo); err != nil {
// 		return nil, errors.Wrap(err, "failed to update repository metadata")
// 	}

// 	return repo, nil
// }

// // shouldSync determines if repository needs synchronization
// func (s *SyncService) shouldSync(repo *models.Repository) bool {
// 	// Always sync if never synced before
// 	if repo.LastSyncedAt == nil {
// 		return true
// 	}

// 	// Don't sync if currently in progress
// 	if repo.SyncStatus == models.SyncStatusInProgress {
// 		return false
// 	}

// 	// Sync if last sync was more than 1 hour ago
// 	return time.Since(*repo.LastSyncedAt) > time.Hour
// }

// // determineSyncSince determines the date from which to sync commits
// func (s *SyncService) determineSyncSince(repo *models.Repository, requestedSince *time.Time) time.Time {
// 	if requestedSince != nil {
// 		return *requestedSince
// 	}

// 	// if repo.SyncSince != nil {
// 	// 	return repo.SyncSince
// 	// }

// 	// Default to sync commits from the last 30 days
// 	return time.Now().AddDate(0, 0, -30)
// }

// // resetCommitsSince deletes commits after the specified date
// func (s *SyncService) resetCommitsSince(tx repository.Transaction, repositoryID int64, since time.Time) (int, error) {
// 	s.logger.Info("resetting commits since date",
// 		"repository_id", repositoryID,
// 		"since", since)

// 	deletedCount, err := s.DeleteCommitsAfterDate(tx, repositoryID, since)
// 	if err != nil {
// 		return 0, errors.Wrap(err, "failed to delete commits after date")
// 	}

// 	s.logger.Info("commits reset completed",
// 		"repository_id", repositoryID,
// 		"deleted_count", deletedCount)

// 	return deletedCount, nil
// }

// // syncCommits fetches and stores commits from GitHub
// func (s *SyncService) syncCommits(tx repository.Transaction, repositoryID int64, owner, repo string, since time.Time) (int, int, error) {
// 	s.logger.Info("syncing commits",
// 		"repository_id", repositoryID,
// 		"owner", owner,
// 		"repository", repo,
// 		"since", since)

// 	// Fetch commits from GitHub with pagination
// 	var allCommits []models.Commit
// 	page := 1
// 	perPage := 100

// 	for {
// 		commits, err := s.fetchCommitsPage(tx.Context(), owner, repo, since, page, perPage)
// 		if err != nil {
// 			return 0, 0, errors.Wrapf(err, "failed to fetch commits page %d", page)
// 		}

// 		// Convert GitHub commits to database commits
// 		dbCommits := s.convertGitHubCommits(repositoryID, commits)
// 		allCommits = append(allCommits, dbCommits...)

// 		if !hasMore {
// 			break
// 		}

// 		page++

// 		// Safety check to prevent infinite loops
// 		if page > 1000 {
// 			s.logger.Warn("reached maximum page limit for commit sync",
// 				"repository_id", repositoryID,
// 				"pages_processed", page)
// 			break
// 		}
// 	}

// 	if len(allCommits) == 0 {
// 		s.logger.Info("no new commits to sync",
// 			"repository_id", repositoryID)
// 		return 0, 0, nil
// 	}

// 	// Batch insert commits
// 	commitsAdded, commitsUpdated, err := s.batchInsertCommits(tx, allCommits)
// 	if err != nil {
// 		return 0, 0, errors.Wrap(err, "failed to batch insert commits")
// 	}

// 	s.logger.Info("commits sync completed",
// 		"repository_id", repositoryID,
// 		"commits_added", commitsAdded,
// 		"commits_updated", commitsUpdated,
// 		"total_commits", len(allCommits))

// 	return commitsAdded, commitsUpdated, nil
// }

// // fetchCommitsPage fetches a single page of commits from GitHub
// func (s *SyncService) fetchCommitsPage(ctx context.Context, owner, repo string, since time.Time, page, perPage int) ([]models.GitHubCommit, error) {

// 	commits, err := s.githubClient.GetCommits(owner, repo, since, page, perPage)

// 	if err != nil {
// 		return nil, errors.Wrap(err, "failed to fetch commits from GitHub")
// 	}

// 	return commits, nil
// }

// // convertGitHubCommits converts GitHub commits to database commits
// func (s *SyncService) convertGitHubCommits(repositoryID int64, githubCommits []models.Commit) []models.Commit {
// 	dbCommits := make([]models.Commit, len(githubCommits))

// 	for i, gc := range githubCommits {
// 		dbCommits[i] = models.Commit{
// 			RepositoryID: gc.ID,
// 			SHA:          gc.SHA,
// 			Message:      gc.Message,
// 			AuthorName:   gc.AuthorName,
// 			AuthorEmail:  gc.AuthorEmail,
// 			CommitDate:   gc.CommitDate,
// 			URL:          gc.URL,
// 			CreatedAt:    time.Now(),
// 		}
// 	}

// 	return dbCommits
// }

// // batchInsertCommits inserts commits in batches with upsert logic
// func (s *SyncService) batchInsertCommits(tx repository.Transaction, commits []models.Commit) (int, int, error) {
// 	if len(commits) == 0 {
// 		return 0, 0, nil
// 	}

// 	batchOp := &repository.BatchOperation{
// 		BatchSize: s.batchSize,
// 		Logger:    s.logger,
// 	}

// 	var totalAdded, totalUpdated int

// 	// Convert commits to interface{} slice for batch processing
// 	items := make([]interface{}, len(commits))
// 	for i, commit := range commits {
// 		items[i] = commit
// 	}

// 	err := batchOp.ProcessInBatches(tx, items, func(tx repository.Transaction, batch []interface{}) error {
// 		// Convert back to commits
// 		batchCommits := make([]models.Commit, len(batch))
// 		for i, item := range batch {
// 			batchCommits[i] = item.(models.Commit)
// 		}

// 		added, updated, err := s.BatchInsertCommits(tx, batchCommits)
// 		if err != nil {
// 			return errors.Wrap(err, "failed to insert commit batch")
// 		}

// 		totalAdded += added
// 		totalUpdated += updated
// 		return nil
// 	})

// 	if err != nil {
// 		return 0, 0, errors.Wrap(err, "failed to process commit batches")
// 	}

// 	return totalAdded, totalUpdated, nil
// }

// // updateCommitStatistics calculates and updates repository commit statistics
// func (s *SyncService) updateCommitStatistics(tx repository.Transaction, repositoryID int64) error {
// 	stats, err := s.calculateCommitStats(tx, repositoryID)
// 	if err != nil {
// 		return errors.Wrap(err, "failed to calculate commit statistics")
// 	}

// 	if err := s.UpdateCommitStats(tx, repositoryID, *stats); err != nil {
// 		return errors.Wrap(err, "failed to update commit statistics")
// 	}

// 	s.logger.Info("commit statistics updated",
// 		"repository_id", repositoryID,
// 		"total_commits", stats.TotalCommits,
// 		"unique_authors", stats.UniqueAuthors)

// 	return nil
// }

// // calculateCommitStats calculates commit statistics for a repository
// func (s *SyncService) calculateCommitStats(tx repository.Transaction, repositoryID int64) (*models.CommitStats, error) {
// 	query := `
// 		SELECT
// 			COUNT(*) as total_commits,
// 			COUNT(DISTINCT author_email) as unique_authors,
// 			MIN(commit_date) as first_commit_date,
// 			MAX(commit_date) as last_commit_date
// 		FROM commits
// 		WHERE repository_id = $1`

// 	var stats models.CommitStats
// 	err := tx.QueryRow(query, repositoryID).Scan(
// 		&stats.TotalCommits,
// 		&stats.UniqueAuthors,
// 		&stats.FirstCommitDate,
// 		&stats.LastCommitDate,
// 	)

// 	if err != nil {
// 		return nil, errors.Wrap(err, "failed to calculate commit statistics")
// 	}

// 	return &stats, nil
// }

// // BulkRepositorySync syncs multiple repositories within individual transactions
// func (s *SyncService) BulkRepositorySync(ctx context.Context, requests []SyncRepositoryRequest) ([]*SyncRepositoryResponse, error) {
// 	s.logger.Info("starting bulk repository sync",
// 		"repository_count", len(requests))

// 	responses := make([]*SyncRepositoryResponse, 0, len(requests))
// 	var errors []error

// 	for i, req := range requests {
// 		s.logger.Info("syncing repository in bulk",
// 			"index", i+1,
// 			"total", len(requests),
// 			"owner", req.Owner,
// 			"repository", req.Repository)

// 		response, err := s.FullRepositorySync(ctx, req)
// 		if err != nil {
// 			s.logger.Error("bulk sync failed for repository",
// 				"owner", req.Owner,
// 				"repository", req.Repository,
// 				"error", err)
// 			errors = append(errors, fmt.Errorf("failed to sync %s/%s: %w", req.Owner, req.Repository, err))
// 			continue
// 		}

// 		responses = append(responses, response)
// 	}

// 	s.logger.Info("bulk repository sync completed",
// 		"successful_syncs", len(responses),
// 		"failed_syncs", len(errors),
// 		"total_repositories", len(requests))

// 	if len(errors) > 0 {
// 		// Return partial success with error details
// 		return responses, fmt.Errorf("bulk sync completed with %d errors: %v", len(errors), errors)
// 	}

// 	return responses, nil
// }

// // ResetRepositorySync resets a repository's sync point within a transaction
// func (s *SyncService) ResetRepositorySync(ctx context.Context, owner, repo string, resetDate time.Time) error {
// 	s.logger.Info("resetting repository sync",
// 		"owner", owner,
// 		"repository", repo,
// 		"reset_date", resetDate)

// 	return s.txManager.WithTransaction(ctx, "reset_repository_sync", func(tx repository.Transaction) error {
// 		// Get repository with lock
// 		repository, err := s.repoRepo.GetRepositoryForUpdate(tx, owner, repo)
// 		if err != nil {
// 			return errors.Wrap(err, "failed to get repository for reset")
// 		}

// 		// Delete commits after reset date
// 		deletedCount, err := s.DeleteCommitsAfterDate(tx, repository.ID, resetDate)
// 		if err != nil {
// 			return errors.Wrap(err, "failed to delete commits after reset date")
// 		}

// 		// Update repository sync_since date
// 		repository.SyncSince = resetDate
// 		if err := s.repoRepo.UpdateRepository(tx, repository); err != nil {
// 			return errors.Wrap(err, "failed to update repository sync_since")
// 		}

// 		// Update sync status to pending for next sync
// 		if err := s.repoRepo.UpdateRepositorySyncStatus(tx, repository.ID, models.SyncStatusPending, time.Now()); err != nil {
// 			return errors.Wrap(err, "failed to update sync status")
// 		}

// 		s.logger.Info("repository sync reset completed",
// 			"repository_id", repository.ID,
// 			"deleted_commits", deletedCount,
// 			"reset_date", resetDate)

// 		return nil
// 	})
// }

// // GetSyncStatus returns the current sync status of a repository
// func (s *SyncService) GetSyncStatus(ctx context.Context, owner, repo string) (*models.SyncStatusResponse, error) {
// 	var response *models.SyncStatusResponse

// 	err := s.txManager.WithTransaction(ctx, "get_sync_status", func(tx repository.Transaction) error {
// 		repository, err := s.repoRepo.GetRepositoryForUpdate(tx, owner, repo)
// 		if err != nil {
// 			return errors.Wrap(err, "failed to get repository")
// 		}

// 		stats, err := s.calculateCommitStats(tx, repository.ID)
// 		if err != nil {
// 			return errors.Wrap(err, "failed to calculate commit stats")
// 		}

// 		response = &models.SyncStatusResponse{
// 			Repository:   repository,
// 			CommitStats:  stats,
// 			SyncStatus:   repository.SyncStatus,
// 			LastSyncedAt: repository.LastSyncedAt,
// 			SyncSince:    &repository.SyncSince,
// 		}

// 		return nil
// 	})

// 	if err != nil {
// 		return nil, errors.Wrap(err, "failed to get sync status")
// 	}

// 	return response, nil
// }

// func (s *SyncService) BatchInsertCommits(tx repository.Transaction, commits []models.Commit) (int, int, error) {
// 	if len(commits) == 0 {
// 		return 0, 0, nil
// 	}

// 	// Use PostgreSQL's UPSERT (INSERT ... ON CONFLICT) for better performance
// 	query := `
// 		INSERT INTO commits (repository_id, sha, message, author_name, author_email, commit_date, url, created_at)
// 		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
// 		ON CONFLICT (repository_id, sha)
// 		DO UPDATE SET
// 			message = EXCLUDED.message,
// 			author_name = EXCLUDED.author_name,
// 			author_email = EXCLUDED.author_email,
// 			commit_date = EXCLUDED.commit_date,
// 			url = EXCLUDED.url,
// 			updated_at = NOW()
// 		RETURNING (xmax = 0) AS inserted`

// 	var added, updated int

// 	for _, commit := range commits {
// 		var inserted bool
// 		err := tx.QueryRow(query,
// 			commit.RepositoryID, commit.SHA, commit.Message,
// 			commit.AuthorName, commit.AuthorEmail, commit.CommitDate,
// 			commit.URL, commit.CreatedAt,
// 		).Scan(&inserted)

// 		if err != nil {
// 			return added, updated, errors.Wrap(err, "failed to upsert commit")
// 		}

// 		if inserted {
// 			added++
// 		} else {
// 			updated++
// 		}
// 	}

// 	s.logger.Debug("batch commit insert completed",
// 		"batch_size", len(commits),
// 		"added", added,
// 		"updated", updated)

// 	return added, updated, nil
// }

// func (s *SyncService) DeleteCommitsAfterDate(tx repository.Transaction, repositoryID int64, date time.Time) (int, error) {
// 	query := `DELETE FROM commits WHERE repository_id = $1 AND commit_date >= $2`

// 	result, err := tx.Exec(query, repositoryID, date)
// 	if err != nil {
// 		return 0, errors.Wrap(err, "failed to delete commits after date")
// 	}

// 	rowsAffected, err := result.RowsAffected()
// 	if err != nil {
// 		return 0, errors.Wrap(err, "failed to get rows affected")
// 	}

// 	s.logger.Info("commits deleted after date",
// 		"repository_id", repositoryID,
// 		"date", date,
// 		"deleted_count", rowsAffected)

// 	return int(rowsAffected), nil
// }

// func (s *SyncService) UpdateCommitStats(tx repository.Transaction, repositoryID int64, stats models.CommitStats) error {
// 	// Update or insert commit statistics (could be a separate table)
// 	query := `
// 		INSERT INTO repository_stats (repository_id, total_commits, unique_authors, first_commit_date, last_commit_date, updated_at)
// 		VALUES ($1, $2, $3, $4, $5, $6)
// 		ON CONFLICT (repository_id)
// 		DO UPDATE SET
// 			total_commits = EXCLUDED.total_commits,
// 			unique_authors = EXCLUDED.unique_authors,
// 			first_commit_date = EXCLUDED.first_commit_date,
// 			last_commit_date = EXCLUDED.last_commit_date,
// 			updated_at = EXCLUDED.updated_at`

// 	_, err := tx.Exec(query,
// 		repositoryID, stats.TotalCommits, stats.UniqueAuthors,
// 		stats.FirstCommitDate, stats.LastCommitDate, time.Now())

// 	if err != nil {
// 		return errors.Wrap(err, "failed to update commit statistics")
// 	}

// 	return nil
// }
