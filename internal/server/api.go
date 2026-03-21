package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dvflw/mantle/internal/auth"
	"github.com/dvflw/mantle/internal/workflow"
)

// executionSummary is the JSON representation of an execution in list responses.
type executionSummary struct {
	ID          string  `json:"id"`
	Workflow    string  `json:"workflow"`
	Version     int     `json:"version"`
	Status      string  `json:"status"`
	StartedAt   *string `json:"started_at"`
	CompletedAt *string `json:"completed_at,omitempty"`
}

// executionDetail is the JSON representation of a single execution with steps.
type executionDetail struct {
	ID          string        `json:"id"`
	Workflow    string        `json:"workflow"`
	Version     int           `json:"version"`
	Status      string        `json:"status"`
	StartedAt   *string       `json:"started_at"`
	CompletedAt *string       `json:"completed_at,omitempty"`
	Steps       []stepSummary `json:"steps"`
}

// stepSummary is the JSON representation of a step execution.
type stepSummary struct {
	Name        string  `json:"name"`
	Status      string  `json:"status"`
	Error       string  `json:"error,omitempty"`
	StartedAt   *string `json:"started_at"`
	CompletedAt *string `json:"completed_at,omitempty"`
}

// handleListExecutions handles GET /api/v1/executions with query param filters.
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

	executions := []executionSummary{}
	for rows.Next() {
		var id, wfName, wfStatus string
		var version int
		var startedAt, completedAt *time.Time
		if err := rows.Scan(&id, &wfName, &version, &wfStatus, &startedAt, &completedAt); err != nil {
			s.Logger.Error("scanning execution row", "error", err)
			writeJSONError(w, "internal server error", http.StatusInternalServerError)
			return
		}

		exec := executionSummary{
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

	writeJSON(w, http.StatusOK, map[string]any{"executions": executions})
}

// handleGetExecution handles GET /api/v1/executions/{id} with step details.
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

	detail := executionDetail{
		ID:       execID,
		Workflow: workflowName,
		Version:  version,
		Status:   status,
		Steps:    []stepSummary{},
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

		step := stepSummary{
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
	writeJSON(w, http.StatusOK, map[string]any{"workflows": workflows})
}

// handleGetWorkflow handles GET /api/v1/workflows/{name} — returns latest version.
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

	var def json.RawMessage = content
	writeJSON(w, http.StatusOK, map[string]any{
		"name":    name,
		"version": version,
		"definition": def,
	})
}

// handleListWorkflowVersions handles GET /api/v1/workflows/{name}/versions.
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
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "versions": versions})
}

// handleGetWorkflowVersion handles GET /api/v1/workflows/{name}/versions/{version}.
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

	var def json.RawMessage = content
	writeJSON(w, http.StatusOK, map[string]any{
		"name":       name,
		"version":    version,
		"definition": def,
	})
}
