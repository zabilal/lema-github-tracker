package models

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

func (r *SyncRepositoryRequest) Validate() error {
	if strings.TrimSpace(r.Owner) == "" {
		return fmt.Errorf("owner is required")
	}
	if strings.TrimSpace(r.Repository) == "" {
		return fmt.Errorf("repository is required")
	}
	if len(r.Owner) > 100 {
		return fmt.Errorf("owner name too long (max 100 characters)")
	}
	if len(r.Repository) > 100 {
		return fmt.Errorf("repository name too long (max 100 characters)")
	}
	return nil
}

type ResetSyncRequest struct {
	FullName string    `json:"full_name" validate:"required"`
	Since    time.Time `json:"since" validate:"required"`
}

func (r *ResetSyncRequest) Validate() error {
	if strings.TrimSpace(r.FullName) == "" {
		return fmt.Errorf("full_name is required")
	}
	if !strings.Contains(r.FullName, "/") {
		return fmt.Errorf("full_name must be in format 'owner/repository'")
	}
	parts := strings.Split(r.FullName, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("invalid full_name format, expected 'owner/repository'")
	}
	if r.Since.IsZero() {
		return fmt.Errorf("since date is required")
	}
	if r.Since.After(time.Now()) {
		return fmt.Errorf("since date cannot be in the future")
	}
	return nil
}

// API Response Models
type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version,omitempty"`
	Uptime    string    `json:"uptime,omitempty"`
}

type SyncResponse struct {
	RepositoryName string    `json:"repository_name"`
	CommitsAdded   int       `json:"commits_added"`
	SyncedAt       time.Time `json:"synced_at"`
}

type CommitResponse struct {
	ID           int       `json:"id"`
	RepositoryID int       `json:"repository_id"`
	SHA          string    `json:"sha"`
	Message      string    `json:"message"`
	AuthorName   string    `json:"author_name"`
	AuthorEmail  string    `json:"author_email"`
	CommitDate   time.Time `json:"commit_date"`
	URL          string    `json:"url"`
	CreatedAt    time.Time `json:"created_at"`
}

type CommitCountResponse struct {
	CommitCount int `json:"commit_count"`
}

type AuthorStatsResponse struct {
	AuthorName   string    `json:"author_name"`
	AuthorEmail  string    `json:"author_email"`
	CommitCount  int       `json:"commit_count"`
	Repositories []string  `json:"repositories"`
	FirstCommit  time.Time `json:"first_commit"`
	LastCommit   time.Time `json:"last_commit"`
}

type RepositoryStatsResponse struct {
	Name            string     `json:"name"`
	FullName        string     `json:"full_name"`
	Description     string     `json:"description,omitempty"`
	Language        string     `json:"language,omitempty"`
	StarsCount      int        `json:"stars_count"`
	ForksCount      int        `json:"forks_count"`
	OpenIssuesCount int        `json:"open_issues_count"`
	WatchersCount   int        `json:"watchers_count"`
	TotalCommits    int        `json:"total_commits"`
	LastCommitDate  *time.Time `json:"last_commit_date"`
	LastSyncedAt    *time.Time `json:"last_synced_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// Pagination models
type PaginationRequest struct {
	Page     int `json:"page" form:"page"`
	PageSize int `json:"page_size" form:"page_size"`
}

func (p *PaginationRequest) Validate() error {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 {
		p.PageSize = 10
	}
	if p.PageSize > 100 {
		p.PageSize = 100
	}
	return nil
}

func (p *PaginationRequest) GetOffset() int {
	return (p.Page - 1) * p.PageSize
}

type PaginatedResponse struct {
	Data       interface{} `json:"data"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	Total      int64       `json:"total"`
	TotalPages int         `json:"total_pages"`
	HasNext    bool        `json:"has_next"`
	HasPrev    bool        `json:"has_prev"`
}

// Error types
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type APIError struct {
	Code    int               `json:"code"`
	Message string            `json:"message"`
	Details string            `json:"details,omitempty"`
	Errors  []ValidationError `json:"errors,omitempty"`
}

func (e APIError) Error() string {
	return e.Message
}

// Helper functions
func SuccessResponse(message string, data interface{}) APIResponse {
	return APIResponse{
		Success: true,
		Message: message,
		Data:    data,
	}
}

func ErrorResponse(message, details string) APIResponse {
	return APIResponse{
		Success: false,
		Message: message,
		Error:   details,
	}
}

func ValidationErrorResponse(errors []ValidationError) APIResponse {
	return APIResponse{
		Success: false,
		Message: "Validation failed",
		Error:   "Invalid request data",
		Data: map[string]interface{}{
			"validation_errors": errors,
		},
	}
}

// JSON helpers
func (r *APIResponse) ToJSON() ([]byte, error) {
	return json.Marshal(r)
}

func FromJSON(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

// Query filters for advanced endpoints
type CommitFilter struct {
	AuthorName  string     `form:"author_name"`
	AuthorEmail string     `form:"author_email"`
	Since       *time.Time `form:"since"`
	Until       *time.Time `form:"until"`
	Message     string     `form:"message"`
}

func (f *CommitFilter) Validate() error {
	if f.Since != nil && f.Until != nil {
		if f.Since.After(*f.Until) {
			return fmt.Errorf("since date cannot be after until date")
		}
	}
	return nil
}

type RepositoryFilter struct {
	Language string `form:"language"`
	MinStars int    `form:"min_stars"`
	MaxStars int    `form:"max_stars"`
}

func (f *RepositoryFilter) Validate() error {
	if f.MinStars < 0 {
		f.MinStars = 0
	}
	if f.MaxStars > 0 && f.MinStars > f.MaxStars {
		return fmt.Errorf("min_stars cannot be greater than max_stars")
	}
	return nil
}

type Repository struct {
	ID              int64      `json:"id"`
	Name            string     `json:"name"`
	FullName        string     `json:"full_name"`
	Description     string     `json:"description,omitempty"`
	URL             string     `json:"url"`
	Language        string     `json:"language"`
	ForksCount      int        `json:"forks_count"`
	StarsCount      int        `json:"stars_count"`
	OpenIssuesCount int        `json:"open_issues_count"`
	WatchersCount   int        `json:"watchers_count"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	LastSyncedAt    *time.Time `json:"last_synced_at"`
	LastCommitSHA   *string    `json:"last_commit_sha"`
	SyncSince       time.Time  `json:"sync_since"`
	CreatedAtDB     time.Time  `json:"created_at_db"`
	UpdatedAtDB     time.Time  `json:"updated_at_db"`
	SyncStatus      SyncStatus `json:"sync_status"`
}

type Commit struct {
	ID           int64     `json:"id"`
	RepositoryID int64     `json:"repository_id"`
	SHA          string    `json:"sha"`
	Message      string    `json:"message"`
	AuthorName   string    `json:"author_name"`
	AuthorEmail  string    `json:"author_email"`
	CommitDate   time.Time `json:"commit_date"`
	URL          string    `json:"url"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type CommitStats struct {
	TotalCommits    int64     `json:"total_commits"`
	UniqueAuthors   int64     `json:"unique_authors"`
	LastCommitDate  time.Time `json:"last_commit_date"`
	FirstCommitDate time.Time `json:"first_commit_date"`
}

type SyncStatus string

const (
	SyncStatusPending    SyncStatus = "pending"
	SyncStatusInProgress SyncStatus = "in_progress"
	SyncStatusCompleted  SyncStatus = "completed"
	SyncStatusFailed     SyncStatus = "failed"
)

type CommitAuthorStats struct {
	AuthorName  string `json:"author_name"`
	AuthorEmail string `json:"author_email"`
	CommitCount int    `json:"commit_count"`
}

// GitHub API response structures
type GitHubRepository struct {
	ID              int       `json:"id"`
	Name            string    `json:"name"`
	FullName        string    `json:"full_name"`
	Description     *string   `json:"description"`
	HTMLURL         string    `json:"html_url"`
	Language        *string   `json:"language"`
	ForksCount      int       `json:"forks_count"`
	StargazersCount int       `json:"stargazers_count"`
	OpenIssuesCount int       `json:"open_issues_count"`
	WatchersCount   int       `json:"watchers_count"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type GitHubCommit struct {
	SHA     string           `json:"sha"`
	Commit  GitHubCommitData `json:"commit"`
	HTMLURL string           `json:"html_url"`
}

type GitHubCommitData struct {
	Message string             `json:"message"`
	Author  GitHubCommitAuthor `json:"author"`
}

type GitHubCommitAuthor struct {
	Name  string    `json:"name"`
	Email string    `json:"email"`
	Date  time.Time `json:"date"`
}

type BatchOperation struct {
	BatchSize int
	Logger    *slog.Logger
}

type SyncStatusResponse struct {
	Repository   *Repository  `json:"repository"`
	CommitStats  *CommitStats `json:"commit_stats"`
	SyncStatus   SyncStatus   `json:"sync_status"`
	LastSyncedAt *time.Time   `json:"last_synced_at"`
	SyncSince    *time.Time   `json:"sync_since"`
}

// SyncRepositoryRequest represents a repository sync request
type SyncRepositoryRequest struct {
	Owner      string     `json:"owner" validate:"required,min=1,max=50"`
	Repository string     `json:"repository" validate:"required,min=1,max=100"`
	SyncSince  *time.Time `json:"sync_since,omitempty"`
	Force      bool       `json:"force,omitempty"`
}

// SyncRepositoryResponse represents the sync operation result
type SyncRepositoryResponse struct {
	Repository     *Repository `json:"repository"`
	CommitsAdded   int                    `json:"commits_added"`
	CommitsUpdated int                    `json:"commits_updated"`
	CommitsDeleted int                    `json:"commits_deleted"`
	SyncDuration   time.Duration          `json:"sync_duration"`
	SyncedAt       time.Time              `json:"synced_at"`
}