package server

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/dvflw/mantle/internal/api/health"
	"github.com/dvflw/mantle/internal/artifact"
	"github.com/dvflw/mantle/internal/audit"
	"github.com/dvflw/mantle/internal/auth"
	"github.com/dvflw/mantle/internal/budget"
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
	Auditor       audit.Emitter
	BudgetStore   *budget.Store
	Address       string
	TLSCertFile   string
	TLSKeyFile    string
	Logger        *slog.Logger

	httpServer   *http.Server
	cron         *CronScheduler
	webhooks     *WebhookHandler
	emailPoller  *EmailTriggerPoller

	worker          *engine.Worker
	reaper          *engine.Reaper
	artifactReaper  *artifact.Reaper

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
	s.emailPoller = NewEmailTriggerPoller(s)
	return s
}

// statusWriter wraps http.ResponseWriter to capture the status code.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

// uuidPattern matches UUID-like path segments.
var uuidPattern = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

// normalizePath collapses UUID segments to {id} to prevent metric cardinality explosion.
func normalizePath(p string) string {
	return uuidPattern.ReplaceAllString(p, "{id}")
}

// metricsMiddleware records HTTP request duration and total count.
func metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		duration := time.Since(start).Seconds()
		path := normalizePath(r.URL.Path)
		status := strconv.Itoa(sw.status)
		metrics.HTTPRequestDuration.WithLabelValues(r.Method, path, status).Observe(duration)
		metrics.HTTPRequestsTotal.WithLabelValues(r.Method, path, status).Inc()
	})
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

	// Budget endpoints.
	mux.HandleFunc("GET /api/v1/budgets", s.handleListBudgets)
	mux.HandleFunc("PUT /api/v1/budgets/{provider}", s.handleSetBudget)
	mux.HandleFunc("DELETE /api/v1/budgets/{provider}", s.handleDeleteBudget)
	mux.HandleFunc("GET /api/v1/budgets/usage", s.handleGetUsage)

	// OpenAPI spec endpoint.
	mux.HandleFunc("GET /api/v1/openapi.json", handleOpenAPISpec)

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

		// Start retention cleanup if configured.
		if cfg.Retention.ExecutionDays > 0 || cfg.Retention.AuditDays > 0 {
			go func() {
				ticker := time.NewTicker(24 * time.Hour)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						if _, err := engine.CleanupExecutions(ctx, s.DB, cfg.Retention.ExecutionDays); err != nil {
							s.Logger.Error("retention cleanup failed", "error", err)
						}
						if _, err := engine.CleanupAuditEvents(ctx, s.DB, cfg.Retention.AuditDays); err != nil {
							s.Logger.Error("audit retention cleanup failed", "error", err)
						}
					}
				}
			}()
			s.Logger.Info("retention cleanup scheduled",
				"execution_days", cfg.Retention.ExecutionDays,
				"audit_days", cfg.Retention.AuditDays)
		}

		// Start artifact reaper if the artifact subsystem is configured.
		if s.Engine.ArtifactStore != nil && s.Engine.Storage != nil {
			retention := 24 * time.Hour // default
			if cfg.Storage.Retention != "" {
				if d, err := time.ParseDuration(cfg.Storage.Retention); err == nil && d > 0 {
					retention = d
				}
			}
			s.artifactReaper = &artifact.Reaper{
				Store:   s.Engine.ArtifactStore,
				Storage: s.Engine.Storage,
				Retention:  retention,
				Logger:     s.Logger,
			}
			go func() {
				interval := cfg.Engine.ReaperInterval
				if interval <= 0 {
					interval = 5 * time.Minute
				}
				ticker := time.NewTicker(interval)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						cleaned, err := s.artifactReaper.Sweep(ctx)
						if err != nil {
							s.Logger.Error("artifact reaper sweep failed", "error", err)
						} else if cleaned > 0 {
							s.Logger.Info("artifact reaper cleaned expired artifacts", "count", cleaned)
						}
					}
				}
			}()
			s.Logger.Info("artifact reaper started", "retention", retention)
		}

		// Periodically update queue depth metric.
		go s.pollQueueDepth(ctx, cfg.Engine.ReaperInterval)
	}

	// Health endpoints (registered after engine components so checkers are populated).
	mux.Handle("/healthz", health.HealthzHandler())
	mux.Handle("/readyz", health.ReadyzHandler(s.DB, livenessCheckers...))

	// Wrap with metrics middleware (innermost, runs for every request).
	handler := metricsMiddleware(mux)
	if s.AuthStore != nil {
		if s.Auditor != nil {
			handler = auth.AuthMiddleware(s.AuthStore, s.OIDCValidator, handler, s.Auditor)
		} else {
			handler = auth.AuthMiddleware(s.AuthStore, s.OIDCValidator, handler)
		}
	}

	// Apply rate limiting (after auth so rate limit keys can use API key prefix).
	rl := NewRateLimiter(100, 200)
	handler = rl.Middleware(handler)

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

	// Start email trigger poller.
	if err := s.emailPoller.Start(ctx); err != nil {
		s.cron.Stop()
		return fmt.Errorf("starting email trigger poller: %w", err)
	}
	s.Logger.Info("email trigger poller started")

	// Start HTTP server.
	errCh := make(chan error, 1)
	go func() {
		if s.TLSCertFile != "" && s.TLSKeyFile != "" {
			s.Logger.Info("server listening with TLS", "address", s.Address)
			if err := s.httpServer.ListenAndServeTLS(s.TLSCertFile, s.TLSKeyFile); err != nil && err != http.ErrServerClosed {
				errCh <- err
			}
		} else {
			slog.Warn("API server running without TLS — use a reverse proxy for production or configure tls.cert_file and tls.key_file")
			s.Logger.Info("server listening", "address", s.Address)
			if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errCh <- err
			}
		}
	}()

	// Wait for shutdown signal.
	select {
	case <-ctx.Done():
		s.Logger.Info("shutting down...")
	case err := <-errCh:
		s.emailPoller.Stop()
		return err
	}

	// Graceful shutdown: stop accepting new requests, wait for in-flight.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	s.cron.Stop()
	s.Logger.Info("cron scheduler stopped")

	s.emailPoller.Stop()
	s.Logger.Info("email trigger poller stopped")

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}
	s.Logger.Info("server stopped")

	// Wait for in-flight executions.
	s.waitForExecutions(shutdownCtx)

	return nil
}

// executeWorkflow runs a workflow in the background, tracking it for graceful shutdown.
// The parent context is used to propagate authentication (team/user) information.
func (s *Server) executeWorkflow(parent context.Context, workflowName string, version int, inputs map[string]any) (string, error) {
	// Propagate auth context from the original request so team scoping is preserved.
	bgCtx := context.Background()
	if u := auth.UserFromContext(parent); u != nil {
		bgCtx = auth.WithUser(bgCtx, u)
	}
	ctx, cancel := context.WithCancel(bgCtx)

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
//
//	@Summary      Trigger a workflow execution
//	@Description  Triggers the latest applied version of the named workflow. Returns a 202 Accepted response with the new execution ID.
//	@Tags         executions
//	@Param    workflow  path  string  true  "Workflow name"
//	@Success  202  {object}  RunResponse
//	@Failure  400  {object}  ErrorResponse
//	@Failure  404  {object}  ErrorResponse
//	@Failure  500  {object}  ErrorResponse
//	@Security ApiKeyAuth
//	@Security OIDCAuth
//	@Router   /api/v1/run/{workflow} [post]
func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit

	workflowName := r.PathValue("workflow")
	if workflowName == "" {
		http.Error(w, `{"error":"workflow name required"}`, http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	version, err := getLatestVersion(ctx, s.DB, workflowName)
	if err != nil || version == 0 {
		writeJSONError(w, "workflow not found", http.StatusNotFound)
		return
	}

	execID, err := s.executeWorkflow(r.Context(), workflowName, version, nil)
	if err != nil {
		s.Logger.Error("workflow execution failed", "workflow", workflowName, "error", err)
		writeJSONError(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(w, `{"execution_id":"%s","workflow":"%s","version":%d}`, execID, workflowName, version)
}

// handleCancel cancels a running execution via the API.
//
//	@Summary      Cancel a running execution
//	@Description  Sends a cancellation signal to a running execution. The execution may not stop immediately; poll the status endpoint to confirm.
//	@Tags         executions
//	@Param    execution  path  string  true  "Execution ID (UUID)"
//	@Success  200  {object}  CancelResponse
//	@Failure  404  {object}  ErrorResponse
//	@Failure  500  {object}  ErrorResponse
//	@Security ApiKeyAuth
//	@Security OIDCAuth
//	@Router   /api/v1/cancel/{execution} [post]
func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	execID := r.PathValue("execution")

	// Update DB status first (with team_id check) to prevent cross-tenant cancellation.
	teamID := auth.TeamIDFromContext(r.Context())
	result, err := s.DB.ExecContext(r.Context(),
		`UPDATE workflow_executions SET status = 'cancelled', completed_at = NOW(), updated_at = NOW()
		 WHERE id = $1 AND status IN ('pending', 'running') AND team_id = $2`, execID, teamID)
	if err != nil {
		s.Logger.Error("cancel: db update failed", "execution", execID, "error", err)
		writeJSONError(w, "internal error", http.StatusInternalServerError)
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		writeJSONError(w, "execution not found or not cancellable", http.StatusNotFound)
		return
	}

	// Only cancel the in-memory context after confirming team ownership via DB.
	s.mu.Lock()
	cancel, ok := s.running[execID]
	s.mu.Unlock()
	if ok {
		cancel()
	}

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
