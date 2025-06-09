package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	_ "github-service/docs"
	"github-service/internal/config"
	"github-service/internal/database"
	"github-service/internal/github"
	"github-service/internal/repository"
	"github-service/internal/scheduler"
	"github-service/internal/server"
	"github-service/internal/service"
	"github-service/pkg/logger"
)

// @title Lema GitHub Service API
// @version 1.0
// @description An API for managing GitHub repositories and analyzing commit data
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.url http://www.swagger.io/support
// @contact.email support@swagger.io

// @license.name MIT
// @license.url https://opensource.org/licenses/MIT

// @host localhost:8080
// @BasePath /api/v1
// @schemes http https

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize logger
	logger := logger.New(cfg.LogLevel)

	// Initialize database
	db, err := database.New(cfg.DatabaseURL)
	if err != nil {
		logger.Fatal("Failed to connect to database", "error", err)
	}
	defer db.Close()

	// Run migrations
	if err := database.RunMigrations(db); err != nil {
		logger.Fatal("Failed to run migrations", "error", err)
	}

	// Initialize GitHub client
	githubClient := github.NewClient(cfg.GitHubToken)

	// Initialize Rate Limit Handler
	rateLimitHandler := github.NewRateLimitHandler(cfg, logger)

	// Initialize repository
	repo := repository.New(db, logger)

	// Initialize service
	githubService := service.NewGitHubService(
		githubClient,
		repo,
		logger,
		rateLimitHandler,
		repository.NewTransactionManager(db, logger),
	)

	// Initialize scheduler
	scheduler := scheduler.New(githubService, logger)

	// // Start scheduler
	// ctx, cancel := context.WithCancel(context.Background())
	// go scheduler.Start(ctx, cfg.SyncInterval)

	// Initialize HTTP server
	serverConfig := server.Config{
		Port:         cfg.ServerPort,
		ReadTimeout:  cfg.ServerReadTimeout,
		WriteTimeout: cfg.ServerWriteTimeout,
		IdleTimeout:  cfg.ServerIdleTimeout,
	}
	httpServer := server.New(githubService, serverConfig, logger)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start components concurrently
	var wg sync.WaitGroup

	// Start scheduler
	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Info("Starting scheduler", "interval", cfg.SyncInterval.String())
		scheduler.Start(ctx, cfg.SyncInterval)
		logger.Info("Scheduler stopped")
	}()

	// Start HTTP server
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := httpServer.Start(); err != nil {
			logger.Error("HTTP server error", "error", err)
		}
	}()

	// Seed with chromium repository data (non-blocking)
	go func() {
		logger.Info("Seeding with chromium repository data...")
		seedCtx, seedCancel := context.WithTimeout(ctx, 10*time.Minute)
		defer seedCancel()

		tm := repository.NewTransactionManager(db, logger)

		err := tm.WithTransaction(seedCtx, "seed_chromium_repository", func(tx repository.Transaction) error {
			return githubService.FetchAndStoreRepository(seedCtx, tx, "chromium", "chromium")
		})

		if err != nil {
			logger.Error("Failed to seed chromium data", "error", err)
		} else {
			logger.Info("Successfully seeded chromium data")
		}
	}()

	logger.Info("Application started successfully",
		"server_port", cfg.ServerPort,
		"sync_interval", cfg.SyncInterval.String(),
		"log_level", cfg.LogLevel)

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("Shutting down gracefully...")
	// cancel()
	// time.Sleep(2 * time.Second) // Allow graceful shutdown

	// Cancel context to stop all goroutines
	cancel()

	// Shutdown HTTP server with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("Failed to shutdown HTTP server gracefully", "error", err)
	}

	// Wait for all goroutines to finish with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("All components shut down successfully")
	case <-time.After(30 * time.Second):
		logger.Error("Timeout waiting for components to shut down")
	}

	logger.Info("Application shutdown complete")
}
