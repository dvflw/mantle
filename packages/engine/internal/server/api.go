package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dvflw/mantle/internal/audit"
	"github.com/dvflw/mantle/internal/auth"
	"github.com/dvflw/mantle/internal/budget"
	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/workflow"
)

// ExecutionSummary is the JSON representation of an execution in list responses.
type ExecutionSummary struct {
	ID          string  `json:"id"`
	Workflow    string  `json:"workflow"`
	Version     int     `json:"version"`
	Status      string  `json:"status"`
	StartedAt   *string `json:"started_at,omitempty"`
	CompletedAt *string `json:"completed_at,omitempty"`
}

// ExecutionDetail is the JSON representation of a single execution with steps.
type ExecutionDetail struct {
	ID          string        `json:"id"`
	Workflow    string        `json:"workflow"`
	Version     int           `json:"version"`
	Status      string        `json:"status"`
	StartedAt   *string       `json:"started_at,omitempty"`
	CompletedAt *string       `json:"completed_at,omitempty"`
	Steps       []StepSummary `json:"steps"`
}

// StepSummary is the JSON representation of a step execution.
type StepSummary struct {
	Name        string  `json:"name"`
	Status      string  `json:"status"`
	Error       string  `json:"error,omitempty"`
	StartedAt   *string `json:"started_at,omitempty"`
	CompletedAt *string `json:"completed_at,omitempty"`
}

// handleListExecutions handles GET /api/v1/executions with query param filters.
//
//	@Summary      List executions
//	@Description  Returns a paginated list of workflow executions for the authenticated team. Supports filtering by workflow name, status, and age.
//	@Tags         executions
//	@Param    workflow  query  string   false  "Filter by workflow name"
//	@Param    status    query  string   false  "Filter by status"   Enums(pending,running,completed,failed,cancelled)
//	@Param    since     query  string   false  "Filter by age (e.g. 1h, 7d)"
//	@Param    limit     query  integer  false  "Max results (default 20)"
//	@Success  200  {object}  ExecutionListResponse
//	@Failure  400  {object}  ErrorResponse
//	@Failure  500  {object}  ErrorResponse
//	@Security ApiKeyAuth
//	@Security OIDCAuth
//	@Router   /api/v1/executions [get]
func (s *Server) handleListExecutions(w http.ResponseWriter, r *http.Request) {
	workflow := r.URL.Query().Get("workflow")
	status := r.URL.Query().Get("status")
	since := r.URL.Query().Get("since")
	limitStr := r.URL.Query().Get("limit")

	limit := 20
	if limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err != nil || parsed <= 0 {
			writeJSONError(w, "invalid limit parameter", http.StatusBadRequest)
			return
		}
		limit = parsed
	}

	// Validate status.
	if status != "" {
		validStatuses := map[string]bool{
			"pending": true, "running": true, "completed": true,
			"failed": true, "cancelled": true,
		}
		status = strings.ToLower(status)
		if !validStatuses[status] {
			writeJSONError(w, "invalid status: must be one of pending, running, completed, failed, cancelled", http.StatusBadRequest)
			return
		}
	}

	// Build parameterized query.
	teamID := auth.TeamIDFromContext(r.Context())
	query := `SELECT id, workflow_name, workflow_version, status, started_at, completed_at
		 FROM workflow_executions WHERE team_id = $1`
	params := []any{teamID}
	paramIdx := 2

	if workflow != "" {
		query += " AND workflow_name = $" + strconv.Itoa(paramIdx)
		params = append(params, workflow)
		paramIdx++
	}

	if status != "" {
		query += " AND status = $" + strconv.Itoa(paramIdx)
		params = append(params, status)
		paramIdx++
	}

	if since != "" {
		duration, err := parseSinceDuration(since)
		if err != nil {
			writeJSONError(w, fmt.Sprintf("invalid since parameter: %s", err), http.StatusBadRequest)
			return
		}
		cutoff := time.Now().Add(-duration)
		query += " AND started_at >= $" + strconv.Itoa(paramIdx)
		params = append(params, cutoff)
		paramIdx++
	}

	query += " ORDER BY started_at DESC NULLS LAST"
	query += " LIMIT $" + strconv.Itoa(paramIdx)
	params = append(params, limit)

	rows, err := s.DB.QueryContext(r.Context(), query, params...)
	if err != nil {
		s.Logger.Error("querying executions", "error", err)
		writeJSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	executions := []ExecutionSummary{}
	for rows.Next() {
		var id, wfName, wfStatus string
		var version int
		var startedAt, completedAt *time.Time
		if err := rows.Scan(&id, &wfName, &version, &wfStatus, &startedAt, &completedAt); err != nil {
			s.Logger.Error("scanning execution row", "error", err)
			writeJSONError(w, "internal server error", http.StatusInternalServerError)
			return
		}

		exec := ExecutionSummary{
			ID:       id,
			Workflow: wfName,
			Version:  version,
			Status:   wfStatus,
		}
		if startedAt != nil {
			ts := startedAt.Format(time.RFC3339)
			exec.StartedAt = &ts
		}
		if completedAt != nil {
			ts := completedAt.Format(time.RFC3339)
			exec.CompletedAt = &ts
		}
		executions = append(executions, exec)
	}

	if err := rows.Err(); err != nil {
		s.Logger.Error("iterating execution rows", "error", err)
		writeJSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, ExecutionListResponse{Executions: executions})
}

// handleGetExecution handles GET /api/v1/executions/{id} with step details.
//
//	@Summary      Get execution detail
//	@Description  Returns full details of a single execution including all step results.
//	@Tags         executions
//	@Param    id  path  string  true  "Execution ID (UUID)"
//	@Success  200  {object}  ExecutionDetail
//	@Failure  400  {object}  ErrorResponse
//	@Failure  404  {object}  ErrorResponse
//	@Failure  500  {object}  ErrorResponse
//	@Security ApiKeyAuth
//	@Security OIDCAuth
//	@Router   /api/v1/executions/{id} [get]
func (s *Server) handleGetExecution(w http.ResponseWriter, r *http.Request) {
	execID := r.PathValue("id")
	if execID == "" {
		writeJSONError(w, "execution ID required", http.StatusBadRequest)
		return
	}

	// Fetch execution.
	teamID := auth.TeamIDFromContext(r.Context())
	var workflowName, status string
	var version int
	var startedAt, completedAt *time.Time
	err := s.DB.QueryRowContext(r.Context(),
		`SELECT workflow_name, workflow_version, status, started_at, completed_at
		 FROM workflow_executions WHERE id = $1 AND team_id = $2`, execID, teamID,
	).Scan(&workflowName, &version, &status, &startedAt, &completedAt)
	if err != nil {
		writeJSONError(w, fmt.Sprintf("execution %q not found", execID), http.StatusNotFound)
		return
	}

	detail := ExecutionDetail{
		ID:       execID,
		Workflow: workflowName,
		Version:  version,
		Status:   status,
		Steps:    []StepSummary{},
	}
	if startedAt != nil {
		ts := startedAt.Format(time.RFC3339)
		detail.StartedAt = &ts
	}
	if completedAt != nil {
		ts := completedAt.Format(time.RFC3339)
		detail.CompletedAt = &ts
	}

	// Fetch steps.
	rows, err := s.DB.QueryContext(r.Context(),
		`SELECT step_name, status, error, started_at, completed_at
		 FROM step_executions WHERE execution_id = $1
		 ORDER BY created_at ASC`, execID,
	)
	if err != nil {
		s.Logger.Error("querying step executions", "error", err)
		writeJSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var stepName, stepStatus string
		var stepError *string
		var stepStarted, stepCompleted *time.Time
		if err := rows.Scan(&stepName, &stepStatus, &stepError, &stepStarted, &stepCompleted); err != nil {
			s.Logger.Error("scanning step row", "error", err)
			writeJSONError(w, "internal server error", http.StatusInternalServerError)
			return
		}

		step := StepSummary{
			Name:   stepName,
			Status: stepStatus,
		}
		if stepError != nil && *stepError != "" {
			step.Error = *stepError
		}
		if stepStarted != nil {
			ts := stepStarted.Format(time.RFC3339)
			step.StartedAt = &ts
		}
		if stepCompleted != nil {
			ts := stepCompleted.Format(time.RFC3339)
			step.CompletedAt = &ts
		}
		detail.Steps = append(detail.Steps, step)
	}

	if err := rows.Err(); err != nil {
		s.Logger.Error("iterating step rows", "error", err)
		writeJSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, detail)
}

// parseSinceDuration parses duration strings like "1h", "24h", "7d".
func parseSinceDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		numStr := strings.TrimSuffix(s, "d")
		days, err := strconv.Atoi(numStr)
		if err != nil {
			return 0, fmt.Errorf("invalid day count: %s", numStr)
		}
		if days <= 0 {
			return 0, fmt.Errorf("duration must be positive")
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	if d <= 0 {
		return 0, fmt.Errorf("duration must be positive")
	}
	return d, nil
}

// RunResponse is returned when a workflow execution is accepted.
type RunResponse struct {
	ExecutionID string `json:"execution_id"`
	Workflow    string `json:"workflow"`
	Version     int    `json:"version"`
}

// CancelResponse is returned when an execution is cancelled.
type CancelResponse struct {
	ExecutionID string `json:"execution_id"`
	Status      string `json:"status"`
}

// ExecutionListResponse wraps a list of executions.
type ExecutionListResponse struct {
	Executions []ExecutionSummary `json:"executions"`
}

// WorkflowListResponse wraps a list of workflow summaries.
type WorkflowListResponse struct {
	Workflows []workflow.WorkflowSummary `json:"workflows"`
}

// WorkflowDetailResponse is returned for GET /api/v1/workflows/{name}.
type WorkflowDetailResponse struct {
	Name       string          `json:"name"`
	Version    int             `json:"version"`
	Definition json.RawMessage `json:"definition" swaggertype:"object"`
}

// WorkflowVersionListResponse wraps a list of workflow versions.
type WorkflowVersionListResponse struct {
	Name     string                    `json:"name"`
	Versions []workflow.VersionSummary `json:"versions"`
}

// ErrorResponse is the standard error envelope.
type ErrorResponse struct {
	Error string `json:"error"`
}

// UsageResponse is returned for GET /api/v1/budgets/usage.
type UsageResponse struct {
	PeriodStart      string `json:"period_start"`
	Provider         string `json:"provider"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
}

// SetBudgetRequest is the request body for PUT /api/v1/budgets/{provider}.
type SetBudgetRequest struct {
	MonthlyTokenLimit int64  `json:"monthly_token_limit"`
	Enforcement       string `json:"enforcement"` // "hard" or "warn"
}

// StatusResponse is returned for mutations that produce no data payload.
type StatusResponse struct {
	Status string `json:"status"`
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeJSONError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// handleListWorkflows handles GET /api/v1/workflows.
//
//	@Summary      List workflow definitions
//	@Description  Returns all workflow definitions ever applied by the authenticated team.
//	@Tags         workflows
//	@Success  200  {object}  WorkflowListResponse
//	@Failure  500  {object}  ErrorResponse
//	@Security ApiKeyAuth
//	@Security OIDCAuth
//	@Router   /api/v1/workflows [get]
func (s *Server) handleListWorkflows(w http.ResponseWriter, r *http.Request) {
	workflows, err := workflow.ListWorkflows(r.Context(), s.DB)
	if err != nil {
		s.Logger.Error("listing workflows", "error", err)
		writeJSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if workflows == nil {
		workflows = []workflow.WorkflowSummary{}
	}
	writeJSON(w, http.StatusOK, WorkflowListResponse{Workflows: workflows})
}

// handleGetWorkflow handles GET /api/v1/workflows/{name} — returns latest version.
//
//	@Summary      Get latest workflow definition
//	@Description  Returns the latest applied definition for a workflow.
//	@Tags         workflows
//	@Param    name  path  string  true  "Workflow name"
//	@Success  200  {object}  WorkflowDetailResponse
//	@Failure  404  {object}  ErrorResponse
//	@Failure  500  {object}  ErrorResponse
//	@Security ApiKeyAuth
//	@Security OIDCAuth
//	@Router   /api/v1/workflows/{name} [get]
func (s *Server) handleGetWorkflow(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeJSONError(w, "workflow name required", http.StatusBadRequest)
		return
	}

	content, version, err := workflow.GetLatestContent(r.Context(), s.DB, name)
	if err != nil {
		s.Logger.Error("getting workflow", "error", err)
		writeJSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if content == nil {
		writeJSONError(w, fmt.Sprintf("workflow %q not found", name), http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, WorkflowDetailResponse{
		Name:       name,
		Version:    version,
		Definition: json.RawMessage(content),
	})
}

// handleListWorkflowVersions handles GET /api/v1/workflows/{name}/versions.
//
//	@Summary      List versions of a workflow
//	@Description  Returns all historical versions of a workflow in reverse chronological order.
//	@Tags         workflows
//	@Param    name  path  string  true  "Workflow name"
//	@Success  200  {object}  WorkflowVersionListResponse
//	@Failure  400  {object}  ErrorResponse
//	@Failure  500  {object}  ErrorResponse
//	@Security ApiKeyAuth
//	@Security OIDCAuth
//	@Router   /api/v1/workflows/{name}/versions [get]
func (s *Server) handleListWorkflowVersions(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeJSONError(w, "workflow name required", http.StatusBadRequest)
		return
	}

	versions, err := workflow.GetVersions(r.Context(), s.DB, name)
	if err != nil {
		s.Logger.Error("listing workflow versions", "error", err)
		writeJSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if versions == nil {
		versions = []workflow.VersionSummary{}
	}
	writeJSON(w, http.StatusOK, WorkflowVersionListResponse{Name: name, Versions: versions})
}

// handleGetWorkflowVersion handles GET /api/v1/workflows/{name}/versions/{version}.
//
//	@Summary      Get a specific workflow version
//	@Description  Returns a specific historical version of a workflow definition.
//	@Tags         workflows
//	@Param    name     path  string   true  "Workflow name"
//	@Param    version  path  integer  true  "Version number"
//	@Success  200  {object}  WorkflowDetailResponse
//	@Failure  400  {object}  ErrorResponse
//	@Failure  404  {object}  ErrorResponse
//	@Security ApiKeyAuth
//	@Security OIDCAuth
//	@Router   /api/v1/workflows/{name}/versions/{version} [get]
func (s *Server) handleGetWorkflowVersion(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeJSONError(w, "workflow name required", http.StatusBadRequest)
		return
	}

	versionStr := r.PathValue("version")
	version, err := strconv.Atoi(versionStr)
	if err != nil || version <= 0 {
		writeJSONError(w, "invalid version number", http.StatusBadRequest)
		return
	}

	content, err := workflow.GetVersion(r.Context(), s.DB, name, version)
	if err != nil {
		writeJSONError(w, fmt.Sprintf("workflow %q version %d not found", name, version), http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, WorkflowDetailResponse{
		Name:       name,
		Version:    version,
		Definition: json.RawMessage(content),
	})
}

// handleListBudgets lists AI provider budgets for the authenticated team.
//
//	@Summary      List provider budgets
//	@Description  Returns the token budget configuration for all providers configured by the authenticated team.
//	@Tags         budgets
//	@Success  200  {array}   budget.TeamBudget
//	@Failure  500  {object}  ErrorResponse
//	@Security ApiKeyAuth
//	@Security OIDCAuth
//	@Router   /api/v1/budgets [get]
func (s *Server) handleListBudgets(w http.ResponseWriter, r *http.Request) {
	teamID := auth.TeamIDFromContext(r.Context())
	budgets, err := s.BudgetStore.ListTeamBudgets(r.Context(), teamID)
	if err != nil {
		s.Logger.Error("listing budgets", "error", err)
		writeJSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, budgets)
}

// handleSetBudget sets or updates the token budget for a provider.
//
//	@Summary      Set provider budget
//	@Description  Creates or replaces the monthly token budget for a provider. Enforcement "hard" blocks execution when the limit is reached; "warn" logs a warning only.
//	@Tags         budgets
//	@Param        provider  path  string           true  "Provider name (e.g. openai, bedrock)"
//	@Param        body      body  SetBudgetRequest  true  "Budget configuration"
//	@Success      200  {object}  StatusResponse
//	@Failure  400  {object}  ErrorResponse
//	@Failure  500  {object}  ErrorResponse
//	@Security ApiKeyAuth
//	@Security OIDCAuth
//	@Router   /api/v1/budgets/{provider} [put]
func (s *Server) handleSetBudget(w http.ResponseWriter, r *http.Request) {
	teamID := auth.TeamIDFromContext(r.Context())
	provider := r.PathValue("provider")

	var body SetBudgetRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.Enforcement == "" {
		body.Enforcement = "hard"
	}
	if body.Enforcement != "hard" && body.Enforcement != "warn" {
		writeJSONError(w, "enforcement must be 'hard' or 'warn'", http.StatusBadRequest)
		return
	}

	if err := s.BudgetStore.SetTeamBudget(r.Context(), teamID, provider, body.MonthlyTokenLimit, body.Enforcement); err != nil {
		s.Logger.Error("setting budget", "provider", provider, "error", err)
		writeJSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	actor := "api"
	if u := auth.UserFromContext(r.Context()); u != nil {
		actor = u.ID
	}

	s.Auditor.Emit(r.Context(), audit.Event{
		Timestamp: time.Now(),
		Actor:     actor,
		Action:    audit.ActionBudgetUpdated,
		Resource:  audit.Resource{Type: "team_budget", ID: teamID},
		Metadata: map[string]string{
			"provider":    provider,
			"limit":       fmt.Sprintf("%d", body.MonthlyTokenLimit),
			"enforcement": body.Enforcement,
		},
		TeamID: teamID,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleDeleteBudget removes the token budget for a provider.
//
//	@Summary      Delete provider budget
//	@Description  Removes the budget configuration for a provider. Does not affect in-flight executions.
//	@Tags         budgets
//	@Param        provider  path  string  true  "Provider name"
//	@Success      200  {object}  StatusResponse
//	@Failure  500  {object}  ErrorResponse
//	@Security ApiKeyAuth
//	@Security OIDCAuth
//	@Router   /api/v1/budgets/{provider} [delete]
func (s *Server) handleDeleteBudget(w http.ResponseWriter, r *http.Request) {
	teamID := auth.TeamIDFromContext(r.Context())
	provider := r.PathValue("provider")

	if err := s.BudgetStore.DeleteTeamBudget(r.Context(), teamID, provider); err != nil {
		s.Logger.Error("deleting budget", "provider", provider, "error", err)
		writeJSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	actor := "api"
	if u := auth.UserFromContext(r.Context()); u != nil {
		actor = u.ID
	}
	s.Auditor.Emit(r.Context(), audit.Event{
		Timestamp: time.Now(),
		Actor:     actor,
		Action:    audit.ActionBudgetUpdated,
		Resource:  audit.Resource{Type: "team_budget", ID: teamID},
		Metadata: map[string]string{
			"provider": provider,
			"action":   "deleted",
		},
		TeamID: teamID,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleGetUsage returns token usage for the current billing period.
//
//	@Summary      Get token usage
//	@Description  Returns token usage aggregated by provider for the current billing period (calendar month UTC).
//	@Tags         budgets
//	@Param    provider  query  string  false  "Provider name; omit for total across all providers"
//	@Success  200  {object}  UsageResponse
//	@Failure  500  {object}  ErrorResponse
//	@Security ApiKeyAuth
//	@Security OIDCAuth
//	@Router   /api/v1/budgets/usage [get]
func (s *Server) handleGetUsage(w http.ResponseWriter, r *http.Request) {
	teamID := auth.TeamIDFromContext(r.Context())
	cfg := config.FromContext(r.Context())

	period := budget.CurrentPeriodStart(time.Now(), cfg.Engine.Budget.ResetMode, cfg.Engine.Budget.ResetDay)

	provider := r.URL.Query().Get("provider")
	var usage *budget.ProviderUsage
	var err error
	if provider != "" {
		usage, err = s.BudgetStore.GetUsage(r.Context(), teamID, provider, period)
	} else {
		usage, err = s.BudgetStore.GetTotalUsage(r.Context(), teamID, period)
	}
	if err != nil {
		s.Logger.Error("getting usage", "provider", provider, "error", err)
		writeJSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"period_start":      period.Format("2006-01-02"),
		"provider":          usage.Provider,
		"prompt_tokens":     usage.PromptTokens,
		"completion_tokens": usage.CompletionTokens,
		"total_tokens":      usage.TotalTokens,
	})
}
