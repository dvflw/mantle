package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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
		writeJSONError(w, "no webhook registered for path", http.StatusNotFound)
		return
	}

	// Read request body as trigger payload.
	var payload any
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		http.Error(w, `{"error":"reading body"}`, http.StatusBadRequest)
		return
	}

	// Verify HMAC signature if the trigger has a secret configured.
	if trigger.Secret != "" {
		if !verifyWebhookSignature(body, trigger.Secret, r.Header) {
			writeJSONError(w, "invalid or missing webhook signature", http.StatusForbidden)
			return
		}
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
		writeJSONError(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(w, `{"execution_id":"%s","workflow":"%s","version":%d}`,
		execID, trigger.WorkflowName, trigger.WorkflowVersion)
}

// verifyWebhookSignature checks the HMAC-SHA256 signature of the request body.
// It looks for the signature in the X-Hub-Signature-256 header (GitHub format: "sha256=<hex>")
// or the X-Signature-256 header as an alternative. Returns false if no signature
// header is present or the signature does not match.
func verifyWebhookSignature(body []byte, secret string, headers http.Header) bool {
	// Try X-Hub-Signature-256 first (GitHub convention), then X-Signature-256.
	sigHeader := headers.Get("X-Hub-Signature-256")
	if sigHeader == "" {
		sigHeader = headers.Get("X-Signature-256")
	}
	if sigHeader == "" {
		return false
	}

	// Expect format "sha256=<hex>".
	if !strings.HasPrefix(sigHeader, "sha256=") {
		return false
	}
	sigHex := sigHeader[len("sha256="):]

	sig, err := hex.DecodeString(sigHex)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := mac.Sum(nil)

	return hmac.Equal(sig, expected)
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
