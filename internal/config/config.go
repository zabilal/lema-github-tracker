package config

import (
	"os"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL  string
	GitHubToken  string
	SyncInterval time.Duration
	LogLevel     string
	StartDate    time.Time

	// Server
	ServerPort         string
	ServerReadTimeout  time.Duration
	ServerWriteTimeout time.Duration
	ServerIdleTimeout  time.Duration
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
