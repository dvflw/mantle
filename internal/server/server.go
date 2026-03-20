package server

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/dvflw/mantle/internal/api/health"
	"github.com/dvflw/mantle/internal/auth"
	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/engine"
	"github.com/dvflw/mantle/internal/metrics"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server is the long-running Mantle process that hosts the API,
// cron scheduler, and webhook listener.
type Server struct {
	DB            *sql.DB
	Engine        *engine.Engine
	AuthStore     *auth.Store
	OIDCValidator *auth.OIDCValidator
	Address       string
	Logger        *slog.Logger

	httpServer *http.Server
	cron       *CronScheduler
	webhooks   *WebhookHandler

	worker     *engine.Worker
	reaper     *engine.Reaper

	mu         sync.Mutex
	running    map[string]context.CancelFunc // execution ID → cancel
}

// workerLivenessChecker adapts the Worker to the health.LivenessChecker interface.
type workerLivenessChecker struct {
	w         *engine.Worker
	threshold time.Duration
}

func (c *workerLivenessChecker) IsAlive() bool { return c.w.IsAlive(c.threshold) }
func (c *workerLivenessChecker) Name() string  { return "worker" }

// reaperLivenessChecker adapts the Reaper to the health.LivenessChecker interface.
type reaperLivenessChecker struct {
	r         *engine.Reaper
	threshold time.Duration
}

func (c *reaperLivenessChecker) IsAlive() bool { return c.r.IsAlive(c.threshold) }
func (c *reaperLivenessChecker) Name() string  { return "reaper" }

// New creates a Server with the given configuration.
func New(db *sql.DB, eng *engine.Engine, address string) *Server {
	logger := slog.Default()
	s := &Server{
		DB:      db,
		Engine:  eng,
		Address: address,
		Logger:  logger,
		running: make(map[string]context.CancelFunc),
	}
	s.cron = NewCronScheduler(s)
	s.webhooks = NewWebhookHandler(s)
	return s
}

// Start starts the HTTP server, cron scheduler, and webhook handler.
// It blocks until the context is cancelled.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Collect liveness checkers for readyz.
	var livenessCheckers []health.LivenessChecker

	// Prometheus metrics endpoint.
	mux.Handle("/metrics", promhttp.Handler())

	// Webhook endpoints.
	mux.HandleFunc("/hooks/", s.webhooks.ServeHTTP)

	// API endpoints.
	mux.HandleFunc("POST /api/v1/run/{workflow}", s.handleRun)
	mux.HandleFunc("POST /api/v1/cancel/{execution}", s.handleCancel)
	mux.HandleFunc("GET /api/v1/executions", s.handleListExecutions)
	mux.HandleFunc("GET /api/v1/executions/{id}", s.handleGetExecution)

	// Workflow definition endpoints.
	mux.HandleFunc("GET /api/v1/workflows", s.handleListWorkflows)
	mux.HandleFunc("GET /api/v1/workflows/{name}", s.handleGetWorkflow)
	mux.HandleFunc("GET /api/v1/workflows/{name}/versions", s.handleListWorkflowVersions)
	mux.HandleFunc("GET /api/v1/workflows/{name}/versions/{version}", s.handleGetWorkflowVersion)

	// Start distributed engine components (worker + reaper).
	if cfg := config.FromContext(ctx); cfg != nil {
		claimer := &engine.Claimer{
			DB:            s.DB,
			NodeID:        cfg.Engine.NodeID,
			LeaseDuration: cfg.Engine.StepLeaseDuration,
		}
		s.worker = &engine.Worker{
			Claimer:      claimer,
			StepExecutor: s.Engine.MakeGlobalStepExecutor(),
			PollInterval:       cfg.Engine.WorkerPollInterval,
			MaxBackoff:         cfg.Engine.WorkerMaxBackoff,
			LeaseRenewInterval: cfg.Engine.StepLeaseDuration / 3,
			Logger:             s.Logger,
		}
		go s.worker.Run(ctx)
		s.Logger.Info("worker started", "node_id", cfg.Engine.NodeID)

		s.reaper = &engine.Reaper{
			DB:       s.DB,
			Interval: cfg.Engine.ReaperInterval,
			Logger:   s.Logger,
		}
		go s.reaper.Run(ctx)
		s.Logger.Info("reaper started", "interval", cfg.Engine.ReaperInterval)

		// Register liveness checkers for readyz.
		workerThreshold := 3 * cfg.Engine.WorkerPollInterval
		if workerThreshold < time.Second {
			workerThreshold = 3 * time.Second
		}
		reaperThreshold := 3 * cfg.Engine.ReaperInterval
		livenessCheckers = append(livenessCheckers,
			&workerLivenessChecker{w: s.worker, threshold: workerThreshold},
			&reaperLivenessChecker{r: s.reaper, threshold: reaperThreshold},
		)

		// Periodically update queue depth metric.
		go s.pollQueueDepth(ctx, cfg.Engine.ReaperInterval)
	}

	// Health endpoints (registered after engine components so checkers are populated).
	mux.Handle("/healthz", health.HealthzHandler())
	mux.Handle("/readyz", health.ReadyzHandler(s.DB, livenessCheckers...))

	// Wrap with auth middleware if AuthStore is configured.
	var handler http.Handler = mux
	if s.AuthStore != nil {
		handler = auth.AuthMiddleware(s.AuthStore, s.OIDCValidator, mux)
	}

	s.httpServer = &http.Server{
		Addr:              s.Address,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Start cron scheduler.
	if err := s.cron.Start(ctx); err != nil {
		return fmt.Errorf("starting cron scheduler: %w", err)
	}
	s.Logger.Info("cron scheduler started")

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

	s.cron.Stop()
	s.Logger.Info("cron scheduler stopped")

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}
	s.Logger.Info("server stopped")

	// Wait for in-flight executions.
	s.waitForExecutions(shutdownCtx)

	return nil
}

// executeWorkflow runs a workflow in the background, tracking it for graceful shutdown.
func (s *Server) executeWorkflow(workflowName string, version int, inputs map[string]any) (string, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Create execution record first to get the ID.
	result, err := s.Engine.Execute(ctx, workflowName, version, inputs)
	if err != nil {
		cancel()
		return "", err
	}

	// Track for graceful shutdown (in practice the execution is already done
	// since Execute is synchronous in V1, but this prepares for async V1.1).
	s.mu.Lock()
	s.running[result.ExecutionID] = cancel
	s.mu.Unlock()

	go func() {
		defer cancel()
		defer func() {
			s.mu.Lock()
			delete(s.running, result.ExecutionID)
			s.mu.Unlock()
		}()
		// Execution already completed in the Execute call above.
		// In V1.1 with async execution, this is where we'd wait.
	}()

	return result.ExecutionID, nil
}

func (s *Server) waitForExecutions(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		s.mu.Lock()
		count := len(s.running)
		s.mu.Unlock()
		if count == 0 {
			return
		}
		select {
		case <-ctx.Done():
			s.Logger.Warn("shutdown timeout, cancelling remaining executions", "count", count)
			s.mu.Lock()
			for _, cancel := range s.running {
				cancel()
			}
			s.mu.Unlock()
			return
		case <-ticker.C:
		}
	}
}

// handleRun triggers a workflow execution via the API.
func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	workflowName := r.PathValue("workflow")
	if workflowName == "" {
		http.Error(w, `{"error":"workflow name required"}`, http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	version, err := getLatestVersion(ctx, s.DB, workflowName)
	if err != nil || version == 0 {
		http.Error(w, fmt.Sprintf(`{"error":"workflow %q not found"}`, workflowName), http.StatusNotFound)
		return
	}

	execID, err := s.executeWorkflow(workflowName, version, nil)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(w, `{"execution_id":"%s","workflow":"%s","version":%d}`, execID, workflowName, version)
}

// handleCancel cancels a running execution via the API.
func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	execID := r.PathValue("execution")
	s.mu.Lock()
	cancel, ok := s.running[execID]
	s.mu.Unlock()

	if ok {
		cancel()
	}

	// Also update DB status.
	s.DB.ExecContext(r.Context(),
		`UPDATE workflow_executions SET status = 'cancelled', completed_at = NOW(), updated_at = NOW()
		 WHERE id = $1 AND status IN ('pending', 'running')`, execID)

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"execution_id":"%s","status":"cancelled"}`, execID)
}

// pollQueueDepth periodically queries the count of pending steps and updates
// the Prometheus gauge. It runs on the same interval as the reaper.
func (s *Server) pollQueueDepth(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var count int
			err := s.DB.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM step_executions WHERE status = 'pending'`,
			).Scan(&count)
			if err != nil {
				s.Logger.Error("failed to query queue depth", "error", err)
				continue
			}
			metrics.SetQueueDepth(count)
		}
	}
}

func getLatestVersion(ctx context.Context, db *sql.DB, name string) (int, error) {
	teamID := auth.TeamIDFromContext(ctx)
	var version int
	err := db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version), 0) FROM workflow_definitions WHERE name = $1 AND team_id = $2`, name, teamID,
	).Scan(&version)
	return version, err
}
