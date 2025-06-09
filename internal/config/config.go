package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL   string
	GitHubToken   string
	GitHubBaseUrl string
	SyncInterval  time.Duration
	LogLevel      string
	StartDate     time.Time

	// Server
	ServerPort         string
	ServerReadTimeout  time.Duration
	ServerWriteTimeout time.Duration
	ServerIdleTimeout  time.Duration

	// GitHub API Rate Limit Handling
	GitHubMaxRetries      int
	GitHubRetryBaseDelay  time.Duration
	GitHubRetryMaxDelay   time.Duration
	GitHubRateLimitBuffer time.Duration
}

func Load() (*Config, error) {
	// Load .env file if it exists
	_ = godotenv.Load()

	cfg := &Config{
		DatabaseURL:  getEnv("DATABASE_URL", "postgres://user:password@localhost/github_service?sslmode=disable"),
		GitHubToken:  getEnv("GITHUB_TOKEN", ""),
		SyncInterval: getDurationEnv("SYNC_INTERVAL", time.Hour),
		LogLevel:     getEnv("LOG_LEVEL", "info"),

		ServerPort:         getEnv("PORT", "8080"),
		ServerReadTimeout:  getDurationEnv("SERVER_READ_TIMEOUT", 60*time.Second),
		ServerWriteTimeout: getDurationEnv("SERVER_WRITE_TIMEOUT", 60*time.Second),
		ServerIdleTimeout:  getDurationEnv("SERVER_IDLE_TIMEOUT", 60*time.Second),

		GitHubBaseUrl:         getEnv("GITHUB_BASEURL", "https://api.github.com"),
		GitHubMaxRetries:      getIntEnv("GITHUB_MAX_RETRIES", 3),
		GitHubRetryBaseDelay:  getDurationEnv("GITHUB_RETRY_BASE_DELAY", 1*time.Second),
		GitHubRetryMaxDelay:   getDurationEnv("GITHUB_RETRY_MAX_DELAY", 5*time.Minute),
		GitHubRateLimitBuffer: getDurationEnv("GITHUB_RATE_LIMIT_BUFFER", 5*time.Minute),
	}

	// Parse start date
	startDateStr := getEnv("START_DATE", "2024-01-01T00:00:00Z")
	startDate, err := time.Parse(time.RFC3339, startDateStr)
	if err != nil {
		return nil, err
	}
	cfg.StartDate = startDate

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func getIntEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}
