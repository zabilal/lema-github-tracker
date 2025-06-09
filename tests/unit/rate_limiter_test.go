package unit

import (
	"net/http"
	"testing"
	"time"

	"github-service/internal/config"
	"github-service/internal/github"
	"github-service/pkg/logger"

	"github.com/stretchr/testify/assert"
)

func TestRateLimitHandler(t *testing.T) {
	cfg := &config.Config{
		GitHubMaxRetries:      3,
		GitHubRetryBaseDelay:  100 * time.Millisecond,
		GitHubRetryMaxDelay:   1 * time.Second,
		GitHubRateLimitBuffer: 1 * time.Second,
	}

	log := logger.New("debug")
	handler := github.NewRateLimitHandler(cfg, log)

	t.Run("ExtractRateLimitInfo", func(t *testing.T) {
		resp := &http.Response{
			Header: http.Header{
				"X-Ratelimit-Limit":     []string{"5000"},
				"X-Ratelimit-Remaining": []string{"100"},
				"X-Ratelimit-Reset":     []string{"1620000000"},
			},
		}

		info := handler.ExtractRateLimitInfo(resp)
		assert.Equal(t, 5000, info.Limit)
		assert.Equal(t, 100, info.Remaining)
		assert.Equal(t, time.Unix(1620000000, 0), info.Reset)
	})

	t.Run("ShouldWait", func(t *testing.T) {
		info := github.RateLimitInfo{
			Limit:     5000,
			Remaining: 501, // 10.02% of 5000 (should not wait)
			Reset:     time.Now().Add(1 * time.Hour),
		}

		assert.False(t, handler.ShouldWait(info), "Should not wait when remaining requests > 10%")

		info.Remaining = 500 // Exactly 10% of 5000 (should wait)
		assert.True(t, handler.ShouldWait(info), "Should wait when remaining requests <= 10%")
	})
}
