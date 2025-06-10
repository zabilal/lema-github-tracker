package models

import "net/http"

// RateLimitError represents a rate limit error from the GitHub API
type RateLimitError struct {
    Response *http.Response
    Message  string
}