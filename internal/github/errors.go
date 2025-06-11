package github

import (
	"fmt"
	"github-service/pkg/errors"
	"net/http"
)

var (
    // ErrRateLimitExceeded is returned when GitHub API rate limit is exceeded
    ErrRateLimitExceeded = errors.New(
        errors.ErrorTypeRateLimit,
        "GitHub API rate limit exceeded",
    ).WithDetails("Please wait before making additional requests")

    // ErrRepositoryNotFound is returned when a repository is not found
    ErrRepositoryNotFound = errors.New(
        errors.ErrorTypeNotFound,
        "repository not found",
    )

    // ErrUnauthorized is returned for 401 Unauthorized responses
    ErrUnauthorized = errors.New(
        errors.ErrorTypeUnauthorized,
        "authentication failed",
    )

    // ErrForbidden is returned for 403 Forbidden responses
    ErrForbidden = errors.New(
        errors.ErrorTypeForbidden,
        "insufficient permissions",
    )
)

// APIError represents an error response from the GitHub API
type APIErrors struct {
    StatusCode int    `json:"-"`
    Message    string `json:"message"`
    URL        string `json:"-"`
}

// Error implements the error interface
func (e *APIErrors) Error() string {
    return fmt.Sprintf("GitHub API error (status: %d): %s", e.StatusCode, e.Message)
}

// ToAppError converts an APIError to an application error
func (e *APIErrors) ToAppError() error {
    switch e.StatusCode {
    case http.StatusNotFound:
        return ErrRepositoryNotFound
    case http.StatusUnauthorized:
        return ErrUnauthorized
    case http.StatusForbidden:
        return errors.Wrap(ErrForbidden, errors.ErrorTypeForbidden, e.Message)
    default:
        return errors.New(
            errors.ErrorTypeServer,
            "GitHub API request failed",
        ).WithDetails(e.Message)
    }
}