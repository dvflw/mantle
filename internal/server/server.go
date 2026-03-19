package server

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/dvflw/mantle/internal/api/health"
	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/engine"
)

// Server is the long-running Mantle process that hosts the API,
// distributed worker, and reaper.
type Server struct {
	DB      *sql.DB
	Address string
	Logger  *slog.Logger

	httpServer *http.Server
}

// New creates a Server with the given configuration.
func New(db *sql.DB, address string) *Server {
	return &Server{
		DB:      db,
		Address: address,
		Logger:  slog.Default(),
	}
}

// Start starts the HTTP server, worker loop, and reaper loop.
// It blocks until the context is cancelled.
func (s *Server) Start(ctx context.Context, cfg *config.Config) error {
	mux := http.NewServeMux()

	// Health endpoints.
	mux.Handle("/healthz", health.HealthzHandler())
	mux.Handle("/readyz", health.ReadyzHandler(s.DB))

	s.httpServer = &http.Server{
		Addr:    s.Address,
		Handler: mux,
	}

	// Create claimer for worker.
	claimer := &engine.Claimer{
		DB:            s.DB,
		NodeID:        cfg.Engine.NodeID,
		LeaseDuration: cfg.Engine.StepLeaseDuration,
	}

	// Start worker loop.
	// For now, the StepExecutor is a placeholder that loads workflow context per step.
	// Full integration with the engine's connector/CEL machinery comes in a follow-up task.
	worker := &engine.Worker{
		Claimer: claimer,
		StepExecutor: func(ctx context.Context, stepName string, attempt int) (map[string]any, error) {
			return nil, fmt.Errorf("step executor not yet wired to engine")
		},
		PollInterval:       cfg.Engine.WorkerPollInterval,
		MaxBackoff:         cfg.Engine.WorkerMaxBackoff,
		LeaseRenewInterval: cfg.Engine.StepLeaseDuration / 3,
		Logger:             s.Logger,
	}
	go worker.Run(ctx)
	s.Logger.Info("worker started", "node_id", cfg.Engine.NodeID)

	// Start reaper.
	reaper := &engine.Reaper{
		DB:       s.DB,
		Interval: cfg.Engine.ReaperInterval,
		Logger:   s.Logger,
	}
	go reaper.Run(ctx)
	s.Logger.Info("reaper started", "interval", cfg.Engine.ReaperInterval)

	// Start HTTP server.
	errCh := make(chan error, 1)
	go func() {
		s.Logger.Info("server listening", "address", s.Address)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Wait for shutdown signal.
	select {
	case <-ctx.Done():
		s.Logger.Info("shutting down...")
	case err := <-errCh:
		return err
	}

	// Graceful shutdown: stop accepting new requests, wait for in-flight.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}
	s.Logger.Info("server stopped")

	return nil
}
