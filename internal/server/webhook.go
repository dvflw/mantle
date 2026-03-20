package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// WebhookHandler handles incoming webhook requests and triggers workflow executions.
type WebhookHandler struct {
	server *Server
}

// NewWebhookHandler creates a webhook handler attached to the server.
func NewWebhookHandler(s *Server) *WebhookHandler {
	return &WebhookHandler{server: s}
}

// ServeHTTP handles POST /hooks/<path> by looking up the registered webhook
// trigger and executing the associated workflow.
func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Extract the webhook path from the URL (e.g., /hooks/my-workflow → /hooks/my-workflow).
	path := r.URL.Path
	if !strings.HasPrefix(path, "/hooks/") {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	// Look up trigger.
	trigger, err := LookupWebhookTrigger(r.Context(), h.server.DB, path)
	if err != nil {
		h.server.Logger.Error("webhook: lookup error", "path", path, "error", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	if trigger == nil {
		http.Error(w, fmt.Sprintf(`{"error":"no webhook registered for path %q"}`, path), http.StatusNotFound)
		return
	}

	// Read request body as trigger payload.
	var payload any
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		http.Error(w, `{"error":"reading body"}`, http.StatusBadRequest)
		return
	}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &payload); err != nil {
			// Not JSON — treat as string.
			payload = string(body)
		}
	}

	// Build inputs with trigger context.
	inputs := map[string]any{
		"trigger": map[string]any{
			"type":    "webhook",
			"path":    path,
			"payload": payload,
			"headers": headerMap(r.Header),
		},
	}

	h.server.Logger.Info("webhook: triggering workflow",
		"path", path,
		"workflow", trigger.WorkflowName,
		"version", trigger.WorkflowVersion)

	execID, err := h.server.executeWorkflow(r.Context(), trigger.WorkflowName, trigger.WorkflowVersion, inputs)
	if err != nil {
		h.server.Logger.Error("webhook: execution failed",
			"workflow", trigger.WorkflowName,
			"error", err)
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(w, `{"execution_id":"%s","workflow":"%s","version":%d}`,
		execID, trigger.WorkflowName, trigger.WorkflowVersion)
}

func headerMap(h http.Header) map[string]any {
	result := make(map[string]any, len(h))
	for k, v := range h {
		if len(v) == 1 {
			result[strings.ToLower(k)] = v[0]
		} else {
			result[strings.ToLower(k)] = v
		}
	}
	return result
}
