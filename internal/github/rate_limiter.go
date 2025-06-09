package github

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github-service/internal/config"
	"github-service/pkg/logger"
)

type RateLimitHandler struct {
	config *config.Config
	logger logger.Logger
}

type RateLimitInfo struct {
	Limit     int
	Remaining int
	Reset     time.Time
}

func NewRateLimitHandler(cfg *config.Config, log logger.Logger) *RateLimitHandler {
	return &RateLimitHandler{
		config: cfg,
		logger: log,
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

func (h *RateLimitHandler) RetryOperation(ctx context.Context, operation func() (*http.Response, error)) (*http.Response, error) {
	var resp *http.Response
	var err error

	for attempt := 0; attempt < h.config.GitHubMaxRetries; attempt++ {
		resp, err = operation()

		if err == nil {
			rateLimitInfo := h.ExtractRateLimitInfo(resp)

			if h.ShouldWait(rateLimitInfo) {
				err = h.WaitForReset(ctx, rateLimitInfo)
				if err != nil {
					return nil, fmt.Errorf("rate limit wait interrupted: %w", err)
				}
				// Retry after waiting
				continue
			}

			return resp, nil
		}

		// Exponential backoff for network errors
		backoffDuration := time.Duration(math.Pow(2, float64(attempt))) * h.config.GitHubRetryBaseDelay
		if backoffDuration > h.config.GitHubRetryMaxDelay {
			backoffDuration = h.config.GitHubRetryMaxDelay
		}

		h.logger.Error("GitHub API request failed (Attempt %d): %v. Retrying in %v", attempt+1, err, backoffDuration)

		select {
		case <-time.After(backoffDuration):
			continue
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return nil, fmt.Errorf("failed after %d attempts: %w", h.config.GitHubMaxRetries, err)
}
