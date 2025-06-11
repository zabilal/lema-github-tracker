package utils

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"time"
)

// Config holds the configuration for retry behavior
type Config struct {
	// MaxRetries is the maximum number of retry attempts
	MaxRetries int
	// MinDelay is the minimum delay between retries
	MinDelay time.Duration
	// MaxDelay is the maximum delay between retries
	MaxDelay time.Duration
	// ShouldRetry is a function that determines if an error should be retried
	ShouldRetry func(error) bool
}

// DefaultConfig provides sensible defaults for retry configuration
var DefaultConfig = Config{
	MaxRetries: 3,
	MinDelay:   100 * time.Millisecond,
	MaxDelay:   30 * time.Second,
	ShouldRetry: func(err error) bool {
		// By default, retry on any error
		return err != nil
	},
}

// WithBackoff executes a function with exponential backoff retry logic
func WithBackoff(ctx context.Context, cfg Config, fn func() error) error {
	var lastErr error

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		// Check if context is done
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Execute the function
		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		// Check if we should retry this error
		if !cfg.ShouldRetry(lastErr) {
			return lastErr
		}

		// If we've reached max retries, break the loop
		if attempt == cfg.MaxRetries {
			break
		}

		// Calculate backoff with jitter
		backoff := calculateBackoff(cfg, attempt)

		// Wait for the backoff period or until context is done
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
			// Continue with the next attempt
		}
	}

	return lastErr
}

// calculateBackoff calculates the backoff duration with jitter
func calculateBackoff(cfg Config, attempt int) time.Duration {
	if cfg.MinDelay <= 0 {
		cfg.MinDelay = 100 * time.Millisecond
	}
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = 30 * time.Second
	}

	// Exponential backoff with jitter
	delay := time.Duration(float64(cfg.MinDelay) * math.Pow(2, float64(attempt)))
	if delay > cfg.MaxDelay {
		delay = cfg.MaxDelay
	}

	// Add jitter (up to 25% of the delay)
	jitter := time.Duration(rand.Int63n(int64(delay / 4)))
	return delay + jitter
}

// IsRetryableError checks if an error is likely temporary/retryable
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Add more conditions as needed
	return errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled) ||
		isNetworkError(err)
}

// isNetworkError checks if the error is a network-related error
func isNetworkError(err error) bool {
	// Add more network-related error checks as needed
	return err.Error() == "connection reset by peer" ||
		err.Error() == "connection refused" ||
		err.Error() == "connection timed out"
}