package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github-service/internal/models"
	"github-service/internal/service"
	"github-service/pkg/logger"

	"github.com/gorilla/mux"
	httpSwagger "github.com/swaggo/http-swagger"
)

// @tag.name Health
// @tag.description Health check operations

// @tag.name Repository
// @tag.description Repository management operations

// @tag.name Analytics
// @tag.description Analytics and statistics operations

type GitHubHandler struct {
	service   *service.GitHubService
	logger    logger.Logger
	startTime time.Time
}

func NewGitHubHandler(service *service.GitHubService, logger logger.Logger) *GitHubHandler {
	return &GitHubHandler{
		service:   service,
		logger:    logger,
		startTime: time.Now(),
	}
}

// HealthCheck godoc
// @Summary Health check endpoint
// @Description Get the health status of the GitHub service
// @Tags Health
// @Accept json
// @Produce json
// @Success 200 {object} models.APIResponse{data=models.HealthResponse} "Service is healthy"
// @Router /health [get]
func (h *GitHubHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(h.startTime)

	healthData := models.HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now().UTC(),
		Version:   "1.0.0",
		Uptime:    uptime.String(),
	}

	response := models.SuccessResponse("GitHub Service is healthy", healthData)
	h.writeJSONResponse(w, http.StatusOK, response)
}

// SyncRepository godoc
// @Summary Sync a specific repository
// @Description Fetch and store repository data from GitHub
// @Tags Repository
// @Accept json
// @Produce json
// @Param request body models.SyncRepositoryRequest true "Repository sync request"
// @Success 200 {object} models.APIResponse{data=models.SyncResponse} "Repository synced successfully"
// @Failure 400 {object} models.APIResponse{errors=[]models.ValidationError} "Validation error"
// @Failure 500 {object} models.APIResponse "Internal server error"
// @Router /repositories/sync [post]
func (h *GitHubHandler) SyncRepository(w http.ResponseWriter, r *http.Request) {
	var req models.SyncRepositoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeValidationError(w, "Invalid JSON format", err.Error())
		return
	}

	if err := req.Validate(); err != nil {
		h.writeValidationError(w, "Validation failed", err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	h.logger.Info("Syncing repository via API",
		"owner", req.Owner,
		"repository", req.Repository,
		"remote_addr", r.RemoteAddr)

	startTime := time.Now()
	if err := h.service.FetchAndStoreRepository(ctx, req.Owner, req.Repository); err != nil {
		h.logger.Error("Failed to sync repository", "error", err)
		h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to sync repository", err.Error())
		return
	}

	syncData := models.SyncResponse{
		RepositoryName: fmt.Sprintf("%s/%s", req.Owner, req.Repository),
		SyncedAt:       time.Now().UTC(),
	}

	duration := time.Since(startTime)
	h.logger.Info("Repository synced successfully",
		"repository", syncData.RepositoryName,
		"duration", duration.String())

	response := models.SuccessResponse("Repository synced successfully", syncData)
	h.writeJSONResponse(w, http.StatusOK, response)
}

// GetCommitsByRepository godoc
// @Summary Get commits by repository
// @Description Retrieve commits for a specific repository with pagination and filtering options
// @Tags Repository
// @Accept json
// @Produce json
// @Param owner path string true "Repository owner"
// @Param repo path string true "Repository name"
// @Param page query int false "Page number (default: 1)" default(1)
// @Param page_size query int false "Page size (default: 20, max: 100)" default(20)
// @Param author_name query string false "Filter by author name"
// @Param author_email query string false "Filter by author email"
// @Param message query string false "Filter by commit message (contains)"
// @Param since query string false "Filter commits since date (RFC3339 format)" format(date-time)
// @Param until query string false "Filter commits until date (RFC3339 format)" format(date-time)
// @Success 200 {object} models.APIResponse{data=[]models.CommitResponse} "Commits retrieved successfully"
// @Failure 400 {object} models.APIResponse{errors=[]models.ValidationError} "Validation error"
// @Failure 500 {object} models.APIResponse "Internal server error"
// @Router /repositories/{owner}/{repo}/commits [get]
func (h *GitHubHandler) GetCommitsByRepository(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	owner := vars["owner"]
	repo := vars["repo"]

	if owner == "" || repo == "" {
		h.writeValidationError(w, "Owner and repository are required", "")
		return
	}

	// Parse pagination
	var pagination models.PaginationRequest
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if page, err := strconv.Atoi(pageStr); err == nil {
			pagination.Page = page
		}
	}
	if pageSizeStr := r.URL.Query().Get("page_size"); pageSizeStr != "" {
		if pageSize, err := strconv.Atoi(pageSizeStr); err == nil {
			pagination.PageSize = pageSize
		}
	}
	pagination.Validate()

	// Parse filters
	var filter models.CommitFilter
	filter.AuthorName = r.URL.Query().Get("author_name")
	filter.AuthorEmail = r.URL.Query().Get("author_email")
	filter.Message = r.URL.Query().Get("message")

	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		if since, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			filter.Since = &since
		}
	}
	if untilStr := r.URL.Query().Get("until"); untilStr != "" {
		if until, err := time.Parse(time.RFC3339, untilStr); err == nil {
			filter.Until = &until
		}
	}

	if err := filter.Validate(); err != nil {
		h.writeValidationError(w, "Invalid filter parameters", err.Error())
		return
	}

	repositoryName := fmt.Sprintf("%s/%s", owner, repo)

	// In a real implementation, the service need be extended to support pagination and filtering
	commits, err := h.service.GetCommitsByRepository(repositoryName, pagination.Page, pagination.PageSize)
	if err != nil {
		h.logger.Error("Failed to get commits by repository", "error", err, "repository", repositoryName)
		h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get commits", err.Error())
		return
	}

	// Convert to response model
	commitResponses := make([]models.CommitResponse, len(commits))
	for i, commit := range commits {
		commitResponses[i] = models.CommitResponse{
			ID:           commit.ID,
			RepositoryID: commit.RepositoryID,
			SHA:          commit.SHA,
			Message:      commit.Message,
			AuthorName:   commit.AuthorName,
			AuthorEmail:  commit.AuthorEmail,
			CommitDate:   commit.CommitDate,
			URL:          commit.URL,
			CreatedAt:    commit.CreatedAt,
		}
	}

	response := models.SuccessResponse(
		fmt.Sprintf("Commits for repository %s retrieved successfully", repositoryName),
		commitResponses,
	)
	h.writeJSONResponse(w, http.StatusOK, response)
}

// GetTopCommitAuthors godoc
// @Summary Get top commit authors
// @Description Retrieve the top commit authors across all repositories
// @Tags Analytics
// @Accept json
// @Produce json
// @Param limit query int false "Maximum number of authors to return (default: 10, max: 100)" default(10)
// @Success 200 {object} models.APIResponse{data=[]models.AuthorStatsResponse} "Top authors retrieved successfully"
// @Failure 400 {object} models.APIResponse{errors=[]models.ValidationError} "Validation error"
// @Failure 500 {object} models.APIResponse "Internal server error"
// @Router /analytics/top-authors [get]
func (h *GitHubHandler) GetTopCommitAuthors(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 10 // default limit

	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 100 {
			limit = parsedLimit
		} else if parsedLimit > 100 {
			h.writeValidationError(w, "Limit cannot exceed 100", "")
			return
		}
	}

	authors, err := h.service.GetTopCommitAuthors(limit)
	if err != nil {
		h.logger.Error("Failed to get top commit authors", "error", err)
		h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get top commit authors", err.Error())
		return
	}

	// Convert to enhanced response format
	authorResponses := make([]models.AuthorStatsResponse, len(authors))
	for i, author := range authors {
		authorResponses[i] = models.AuthorStatsResponse{
			AuthorName:  author.AuthorName,
			AuthorEmail: author.AuthorEmail,
			CommitCount: author.CommitCount,
			// Repositories: []string{}, // Would need to be populated from service
			// FirstCommit and LastCommit would also need to be added to the service
		}
	}

	response := models.SuccessResponse("Top commit authors retrieved successfully", authorResponses)
	h.writeJSONResponse(w, http.StatusOK, response)
}

// GetRepositoryStats godoc
// @Summary Get repository statistics
// @Description Retrieve detailed statistics for a specific repository
// @Tags Repository
// @Accept json
// @Produce json
// @Param owner path string true "Repository owner"
// @Param repo path string true "Repository name"
// @Success 200 {object} models.APIResponse{data=models.RepositoryStatsResponse} "Repository statistics retrieved successfully"
// @Failure 400 {object} models.APIResponse{errors=[]models.ValidationError} "Validation error"
// @Failure 500 {object} models.APIResponse "Internal server error"
// @Router /repositories/{owner}/{repo}/stats [get]
func (h *GitHubHandler) GetRepositoryStats(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	owner := vars["owner"]
	repo := vars["repo"]

	if owner == "" || repo == "" {
		h.writeValidationError(w, "Owner and repository are required", "")
		return
	}

	repositoryName := fmt.Sprintf("%s/%s", owner, repo)

	// This would require extending the service to get repository stats
	// For now, we'll return a placeholder response
	statsData, err := h.service.GetRepositoryStats(repositoryName)
	if err != nil {
		h.logger.Error("Failed to get repository stats", "error", err)
		h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get repository stats", err.Error())
		return
	}
	response := models.SuccessResponse(
		fmt.Sprintf("Statistics for repository %s retrieved successfully", repositoryName),
		statsData,
	)
	h.writeJSONResponse(w, http.StatusOK, response)
}

// SyncAllRepositories godoc
// @Summary Sync all repositories
// @Description Synchronize all repositories from the configured GitHub sources
// @Tags Repository
// @Accept json
// @Produce json
// @Success 200 {object} models.APIResponse{data=map[string]interface{}} "All repositories synced successfully"
// @Failure 500 {object} models.APIResponse "Internal server error"
// @Router /repositories/sync-all [post]
func (h *GitHubHandler) SyncAllRepositories(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()

	h.logger.Info("Syncing all repositories via API", "remote_addr", r.RemoteAddr)

	startTime := time.Now()
	if err := h.service.SyncAllRepositories(ctx); err != nil {
		h.logger.Error("Failed to sync all repositories", "error", err)
		h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to sync all repositories", err.Error())
		return
	}

	duration := time.Since(startTime)
	h.logger.Info("All repositories synced successfully", "duration", duration.String())

	syncData := map[string]interface{}{
		"synced_at": time.Now().UTC(),
		"duration":  duration.String(),
	}

	response := models.SuccessResponse("All repositories synced successfully", syncData)
	h.writeJSONResponse(w, http.StatusOK, response)
}

// ResetRepositorySync godoc
// @Summary Reset repository sync
// @Description Reset the synchronization state for a specific repository
// @Tags Repository
// @Accept json
// @Produce json
// @Param request body models.ResetSyncRequest true "Reset sync request"
// @Success 200 {object} models.APIResponse{data=map[string]interface{}} "Repository sync reset successfully"
// @Failure 400 {object} models.APIResponse{errors=[]models.ValidationError} "Validation error"
// @Failure 500 {object} models.APIResponse "Internal server error"
// @Router /repositories/reset-sync [post]
func (h *GitHubHandler) ResetRepositorySync(w http.ResponseWriter, r *http.Request) {
	var req models.ResetSyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeValidationError(w, "Invalid JSON format", err.Error())
		return
	}

	if err := req.Validate(); err != nil {
		h.writeValidationError(w, "Validation failed", err.Error())
		return
	}

	h.logger.Info("Resetting repository sync",
		"full_name", req.FullName,
		"since", req.Since,
		"remote_addr", r.RemoteAddr)

	if err := h.service.ResetRepositorySync(req.FullName, req.Since); err != nil {
		h.logger.Error("Failed to reset repository sync", "error", err)
		h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to reset repository sync", err.Error())
		return
	}

	resetData := map[string]interface{}{
		"repository": req.FullName,
		"reset_to":   req.Since,
		"reset_at":   time.Now().UTC(),
	}

	response := models.SuccessResponse(
		fmt.Sprintf("Repository sync reset for %s", req.FullName),
		resetData,
	)
	h.writeJSONResponse(w, http.StatusOK, response)
}

// Helper methods
func (h *GitHubHandler) writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("Failed to encode JSON response", "error", err)
	}
}

func (h *GitHubHandler) writeErrorResponse(w http.ResponseWriter, statusCode int, message, details string) {
	errorResp := models.ErrorResponse(message, details)
	h.writeJSONResponse(w, statusCode, errorResp)
}

func (h *GitHubHandler) writeValidationError(w http.ResponseWriter, message, details string) {
	errors := []models.ValidationError{
		{
			Field:   "request",
			Message: details,
		},
	}
	errorResp := models.ValidationErrorResponse(errors)
	h.writeJSONResponse(w, http.StatusBadRequest, errorResp)
}

// Setup enhanced routes
func (h *GitHubHandler) SetupRoutes() *mux.Router {
	router := mux.NewRouter()

	// Apply middleware (same as before)
	router.Use(h.RecoveryMiddleware)
	router.Use(h.LoggingMiddleware)
	router.Use(h.CORSMiddleware)

	// Swagger documentation endpoint
	router.PathPrefix("/swagger/").Handler(httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
		httpSwagger.DeepLinking(true),
		httpSwagger.DocExpansion("none"),
		httpSwagger.DomID("swagger-ui"),
	))

	// API routes
	api := router.PathPrefix("/api/v1").Subrouter()

	// Health check
	api.HandleFunc("/health", h.HealthCheck).Methods("GET")

	// Repository operations
	api.HandleFunc("/repositories/sync", h.SyncRepository).Methods("POST")
	api.HandleFunc("/repositories/sync-all", h.SyncAllRepositories).Methods("POST")
	api.HandleFunc("/repositories/reset-sync", h.ResetRepositorySync).Methods("POST")
	api.HandleFunc("/repositories/{owner}/{repo}/commits", h.GetCommitsByRepository).Methods("GET")
	api.HandleFunc("/repositories/{owner}/{repo}/stats", h.GetRepositoryStats).Methods("GET")

	// Analytics
	api.HandleFunc("/analytics/top-authors", h.GetTopCommitAuthors).Methods("GET")

	return router
}

// Middleware methods (same as before)
func (h *GitHubHandler) LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)

		h.logger.Info("HTTP Request",
			"method", r.Method,
			"path", r.URL.Path,
			"query", r.URL.RawQuery,
			"status", wrapped.statusCode,
			"duration", duration.String(),
			"remote_addr", r.RemoteAddr,
			"user_agent", r.Header.Get("User-Agent"),
		)
	})
}

func (h *GitHubHandler) CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (h *GitHubHandler) RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				h.logger.Error("Panic recovered", "error", err)
				h.writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", "")
			}
		}()

		next.ServeHTTP(w, r)
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
