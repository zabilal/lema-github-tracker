package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github-service/internal/handler"
	"github-service/internal/service"
	"github-service/pkg/logger"

	"github.com/gorilla/mux"
)

type Server struct {
	httpServer *http.Server
	handler    *handler.GitHubHandler
	logger     logger.Logger
}

type Config struct {
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

func New(githubService *service.GitHubService, config Config, logger logger.Logger) *Server {
	handler := handler.NewGitHubHandler(githubService, logger)

	server := &Server{
		handler: handler,
		logger:  logger,
	}

	router := handler.SetupRoutes()

	server.httpServer = &http.Server{
		Addr:         ":" + config.Port,
		Handler:      router,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
		IdleTimeout:  config.IdleTimeout,
	}

	return server
}

func (s *Server) Start() error {
	s.logger.Info("Starting HTTP server", "addr", s.httpServer.Addr)

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("failed to start server: %w", err)
	}

	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down HTTP server")

	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown server: %w", err)
	}

	s.logger.Info("HTTP server shutdown complete")
	return nil
}

func (s *Server) GetRouter() *mux.Router {
	return s.handler.SetupRoutes()
}
