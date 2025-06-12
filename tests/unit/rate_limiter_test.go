package unit

import (
	"context"
	"testing"
	"time"

	"github-service/internal/config"
	"github-service/internal/github"
	"github-service/pkg/logger/mocks"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

// testClient wraps a real client but allows overriding GetRateLimit
// We need to use a real client because RateLimitHandler expects a concrete *Client
type testClient struct {
	*github.Client
	getRateLimitFunc func(ctx context.Context) (*github.RateLimitInfo, error)
}

// GetRateLimit overrides the default implementation with our test function
func (c *testClient) GetRateLimit(ctx context.Context) (*github.RateLimitInfo, error) {
	if c.getRateLimitFunc != nil {
		return c.getRateLimitFunc(ctx)
	}
	return c.Client.GetRateLimit(ctx)
}

// Helper function to create a test rate limit handler with a mock rate limit function
func newTestRateLimitHandler(t *testing.T, cfg *config.Config, mockLogger *mocks.MockLogger, getRateLimitFunc func(ctx context.Context) (*github.RateLimitInfo, error)) *github.RateLimitHandler {
	// Create a real client with a test token
	client := &testClient{
		Client:           github.NewClient("test-token"),
		getRateLimitFunc: getRateLimitFunc,
	}
	
	// Create the handler with our test client
	handler := github.NewRateLimitHandler(cfg, mockLogger, client.Client)
	return handler
}

func TestRateLimitHandler_RetryOperation(t *testing.T) {
	ctx := context.Background()

	// Create mock controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock logger
	mockLogger := mocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any(), gomock.Any()).AnyTimes()

	// Create mock config
	cfg := &config.Config{
		GitHubMaxRetries:      3,
		GitHubRetryBaseDelay:  time.Second,
		GitHubRetryMaxDelay:   30 * time.Second,
		GitHubRateLimitBuffer: time.Second,
	}

	// Create handler with mocks
	handler := newTestRateLimitHandler(t, cfg, mockLogger, func(ctx context.Context) (*github.RateLimitInfo, error) {
		return &github.RateLimitInfo{
			Limit:     5000,
			Remaining: 5000, // Start with full rate limit
			Reset:     time.Now().Add(time.Hour),
		}, nil
	})

	t.Run("success on first try", func(t *testing.T) {
		callCount := 0
		err := handler.RetryOperation(ctx, func() error {
			callCount++
			return nil
		})

		assert.NoError(t, err)
		assert.Equal(t, 1, callCount)
	})

		t.Run("retry on rate limit", func(t *testing.T) {
		callCount := 0
		start := time.Now()

		// Configure the handler with a mock rate limit function
		handler := newTestRateLimitHandler(t, cfg, mockLogger, func(ctx context.Context) (*github.RateLimitInfo, error) {
			return &github.RateLimitInfo{
				Limit:     5000,
				Remaining: 0,
				Reset:     time.Now().Add(30 * time.Second),
			}, nil
		})

		operation := func() error {
			callCount++
			if callCount == 1 {
				// First call - simulate rate limit exceeded
				return &github.APIError{
					StatusCode: 403,
					Message:    "API rate limit exceeded",
				}
			}
			return nil
		}

		err := handler.RetryOperation(ctx, operation)

		elapsed := time.Since(start)
		assert.NoError(t, err)
		assert.Equal(t, 2, callCount, "Operation should be called twice (initial + retry)")
		assert.GreaterOrEqual(t, elapsed, time.Second, "Should have waited at least 1 second")
	})

	t.Run("return error after max retries", func(t *testing.T) {
		callCount := 0

		// Create a new handler that always returns rate limit exceeded
		handler := newTestRateLimitHandler(t, cfg, mockLogger, func(ctx context.Context) (*github.RateLimitInfo, error) {
			return &github.RateLimitInfo{
				Limit:     5000,
				Remaining: 0,
				Reset:     time.Now().Add(30 * time.Second),
			}, nil
		})

		err := handler.RetryOperation(ctx, func() error {
			callCount++
			return &github.APIError{
				StatusCode: 403,
				Message:    "API rate limit exceeded",
			}
		})

		assert.Error(t, err)
		assert.Equal(t, 4, callCount, "Operation should be called 4 times (initial + 3 retries)")
		assert.Contains(t, err.Error(), "API rate limit exceeded", "Error should contain rate limit message")
	})
}
