package github

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github-service/internal/config"
	"github-service/pkg/logger"
)

type RateLimitHandler struct {
	config *config.Config
	logger logger.Logger
	client *Client
}

type RateLimitInfo struct {
	Limit     int
	Remaining int
	Reset     time.Time
}

type RateLimitResponse struct {
	Resources struct {
		Core struct {
			Limit     int   `json:"limit"`
			Remaining int   `json:"remaining"`
			Reset     int64 `json:"reset"` // Unix timestamp
		} `json:"core"`
	} `json:"resources"`
}

func NewRateLimitHandler(cfg *config.Config, log logger.Logger, client *Client) *RateLimitHandler {
	return &RateLimitHandler{
		config: cfg,
		logger: log,
		client: client,
	}
}

func (h *RateLimitHandler) ExtractRateLimitInfo(resp *http.Response) RateLimitInfo {
	limit, _ := strconv.Atoi(resp.Header.Get("X-RateLimit-Limit"))
	remaining, _ := strconv.Atoi(resp.Header.Get("X-RateLimit-Remaining"))
	resetTimestamp, _ := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64)

	return RateLimitInfo{
		Limit:     limit,
		Remaining: remaining,
		Reset:     time.Unix(resetTimestamp, 0),
	}
}

func (h *RateLimitHandler) ShouldWait(info RateLimitInfo) bool {
	// Check if remaining requests are critically low
	return info.Remaining <= int(math.Ceil(float64(info.Limit)*0.1))
}

func (h *RateLimitHandler) WaitForReset(ctx context.Context, info RateLimitInfo) error {
	waitDuration := time.Until(info.Reset) + h.config.GitHubRateLimitBuffer

	if waitDuration > 0 {
		h.logger.Info("Rate limit near exhaustion. Waiting for %v", waitDuration)

		select {
		case <-time.After(waitDuration):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// Update the RetryOperation to use the new rate limit check
func (h *RateLimitHandler) RetryOperation(ctx context.Context, operation func() error) error {
	var lastErr error
	retries := h.config.GitHubMaxRetries
	baseDelay := h.config.GitHubRetryBaseDelay

	for attempt := 0; attempt <= retries; attempt++ {
		// Execute the operation
		err := operation()
		if err == nil {
			return nil // Success
		}

		// If it's not a rate limit error, return immediately
		if !isRateLimitError(err) {
			return err
		}
		h.logger.Info("Rate limit error detected, will retry", "attempt", attempt+1, "max_attempts", retries, "error", err)

		// Get current rate limit status
		rateInfo, err := h.client.GetRateLimit(ctx)
		if err != nil {
			h.logger.Warn("Failed to get rate limit info", "error", err)
			return err
		}

		// Check if we should wait
		if !h.ShouldWait(*rateInfo) {
			return err
		}

		// Calculate wait time
		waitTime := rateInfo.Reset.Sub(time.Now()) + time.Second
		if waitTime < 0 {
			waitTime = baseDelay * time.Duration(math.Pow(2, float64(attempt)))
		}

		h.logger.Info("Rate limit exceeded, waiting to retry",
			"wait_time", waitTime,
			"remaining", rateInfo.Remaining,
			"reset", rateInfo.Reset,
		)

		select {
		case <-time.After(waitTime):
			continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return fmt.Errorf("max retries (%d) exceeded: %w", retries, lastErr)
}

// func (h *RateLimitHandler) RetryOperation(ctx context.Context, operation func()  error) (*http.Response, error) {
// 	var resp *http.Response
// 	var err error

// 	for attempt := 0; attempt < h.config.GitHubMaxRetries; attempt++ {
// 		err = operation()

// 		if err == nil {
// 			rateLimitInfo := h.ExtractRateLimitInfo(resp)

// 			if h.ShouldWait(rateLimitInfo) {
// 				err = h.WaitForReset(ctx, rateLimitInfo)
// 				if err != nil {
// 					return nil, fmt.Errorf("rate limit wait interrupted: %w", err)
// 				}
// 				// Retry after waiting
// 				continue
// 			}

// 			return resp, nil
// 		}

// 		// Exponential backoff for network errors
// 		backoffDuration := time.Duration(math.Pow(2, float64(attempt))) * h.config.GitHubRetryBaseDelay
// 		if backoffDuration > h.config.GitHubRetryMaxDelay {
// 			backoffDuration = h.config.GitHubRetryMaxDelay
// 		}

// 		h.logger.Error("GitHub API request failed (Attempt %d): %v. Retrying in %v", attempt+1, err, backoffDuration)

// 		select {
// 		case <-time.After(backoffDuration):
// 			continue
// 		case <-ctx.Done():
// 			return nil, ctx.Err()
// 		}
// 	}

// 	return nil, fmt.Errorf("failed after %d attempts: %w", h.config.GitHubMaxRetries, err)
// }

// isRateLimitError checks if the error is a rate limit error
func isRateLimitError(err error) bool {
	// Check for HTTP 403 status code which typically indicates rate limiting
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "403")
}

// GetRateLimit gets the current rate limit information
func (c *Client) GetRateLimit(ctx context.Context) (*RateLimitInfo, error) {
	url := fmt.Sprintf("%s/rate_limit", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create rate limit request: %w", err)
	}

	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get rate limit: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var rateLimitResp RateLimitResponse
	if err := json.NewDecoder(resp.Body).Decode(&rateLimitResp); err != nil {
		return nil, fmt.Errorf("failed to decode rate limit response: %w", err)
	}

	return &RateLimitInfo{
		Limit:     rateLimitResp.Resources.Core.Limit,
		Remaining: rateLimitResp.Resources.Core.Remaining,
		Reset:     time.Unix(rateLimitResp.Resources.Core.Reset, 0),
	}, nil
}
