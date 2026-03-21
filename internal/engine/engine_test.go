package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dvflw/mantle/internal/workflow"
)

// applyWorkflow stores a workflow definition and returns the version number.
func applyWorkflow(t *testing.T, database *sql.DB, yamlContent []byte) int {
	t.Helper()
	result, err := workflow.ParseBytes(yamlContent)
	if err != nil {
		t.Fatalf("ParseBytes() error: %v", err)
	}
	version, err := workflow.Save(context.Background(), database, result, yamlContent)
	if err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	if version == 0 {
		t.Fatal("Save() returned version 0 (no changes)")
	}
	return version
}

func TestEngine_SimpleHTTPWorkflow(t *testing.T) {
	database := setupTestDB(t)

	// Start a test HTTP server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": "hello from server",
		})
	}))
	defer server.Close()

	// Apply workflow.
	wfYAML := []byte(`name: test-http
description: Test HTTP workflow
steps:
  - name: fetch
    action: http/request
    params:
      method: GET
      url: "` + server.URL + `"
`)
	version := applyWorkflow(t, database, wfYAML)

	// Execute.
	eng, err := New(database)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	result, err := eng.Execute(context.Background(), "test-http", version, nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if result.Status != "completed" {
		t.Errorf("status = %q, want %q (error: %s)", result.Status, "completed", result.Error)
	}

	fetchResult := result.Steps["fetch"]
	if fetchResult.Status != "completed" {
		t.Errorf("fetch status = %q, want %q", fetchResult.Status, "completed")
	}
	if fetchResult.Output["status"] != int64(200) {
		t.Errorf("fetch output.status = %v, want 200", fetchResult.Output["status"])
	}
}

func TestEngine_CELDataPassing(t *testing.T) {
	database := setupTestDB(t)

	// Step 1 returns JSON, step 2 uses it via CEL.
	step1Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"user": "alice",
			"id":   42,
		})
	}))
	defer step1Server.Close()

	var receivedBody map[string]any
	step2Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer step2Server.Close()

	wfYAML := []byte(`name: test-cel-passing
description: Test CEL data passing
inputs:
  greeting:
    type: string
steps:
  - name: get-user
    action: http/request
    params:
      method: GET
      url: "` + step1Server.URL + `"
  - name: post-result
    action: http/request
    params:
      method: POST
      url: "` + step2Server.URL + `"
      body:
        message: "{{ inputs.greeting }}"
`)
	version := applyWorkflow(t, database, wfYAML)

	eng, err := New(database)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	result, err := eng.Execute(context.Background(), "test-cel-passing", version, map[string]any{
		"greeting": "hello world",
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if result.Status != "completed" {
		t.Fatalf("status = %q, want %q (error: %s)", result.Status, "completed", result.Error)
	}

	// Verify the second step received the interpolated value.
	if receivedBody["message"] != "hello world" {
		t.Errorf("step 2 received message = %v, want %q", receivedBody["message"], "hello world")
	}
}

func TestEngine_ConditionalStep_Skipped(t *testing.T) {
	database := setupTestDB(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("conditional step should not have been called")
	}))
	defer server.Close()

	wfYAML := []byte(`name: test-conditional
description: Test conditional skip
steps:
  - name: always-skip
    action: http/request
    if: "false"
    params:
      url: "` + server.URL + `"
`)
	version := applyWorkflow(t, database, wfYAML)

	eng, err := New(database)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	result, err := eng.Execute(context.Background(), "test-conditional", version, nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if result.Status != "completed" {
		t.Errorf("status = %q, want %q", result.Status, "completed")
	}
	if result.Steps["always-skip"].Status != "skipped" {
		t.Errorf("always-skip status = %q, want %q", result.Steps["always-skip"].Status, "skipped")
	}
}

func TestEngine_ConditionalStep_Executed(t *testing.T) {
	database := setupTestDB(t)

	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]any{"done": true})
	}))
	defer server.Close()

	wfYAML := []byte(`name: test-conditional-true
description: Test conditional execution
steps:
  - name: run-me
    action: http/request
    if: "true"
    params:
      url: "` + server.URL + `"
`)
	version := applyWorkflow(t, database, wfYAML)

	eng, err := New(database)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	result, err := eng.Execute(context.Background(), "test-conditional-true", version, nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if !called {
		t.Error("conditional step with if:true should have been called")
	}
	if result.Steps["run-me"].Status != "completed" {
		t.Errorf("run-me status = %q, want %q", result.Steps["run-me"].Status, "completed")
	}
}

func TestEngine_StepFailure(t *testing.T) {
	database := setupTestDB(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	wfYAML := []byte(`name: test-failure
description: Test step failure
steps:
  - name: fail-step
    action: http/request
    params:
      url: "` + server.URL + `"
`)
	version := applyWorkflow(t, database, wfYAML)

	eng, err := New(database)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	result, err := eng.Execute(context.Background(), "test-failure", version, nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if result.Status != "failed" {
		t.Errorf("status = %q, want %q", result.Status, "failed")
	}
	if result.Steps["fail-step"].Status != "failed" {
		t.Errorf("fail-step status = %q, want %q", result.Steps["fail-step"].Status, "failed")
	}
}

func TestEngine_CheckpointRecovery(t *testing.T) {
	database := setupTestDB(t)

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		json.NewEncoder(w).Encode(map[string]any{"call": callCount})
	}))
	defer server.Close()

	wfYAML := []byte(`name: test-checkpoint
description: Test checkpoint recovery
steps:
  - name: step-one
    action: http/request
    params:
      url: "` + server.URL + `"
  - name: step-two
    action: http/request
    params:
      url: "` + server.URL + `"
`)
	version := applyWorkflow(t, database, wfYAML)

	eng, err := New(database)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	// Run normally first.
	result1, err := eng.Execute(context.Background(), "test-checkpoint", version, nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result1.Status != "completed" {
		t.Fatalf("first run status = %q, want %q", result1.Status, "completed")
	}
	if callCount != 2 {
		t.Fatalf("first run call count = %d, want 2", callCount)
	}

	// Simulate a second execution that resumes. We manually create an execution
	// with step-one already completed to test checkpoint recovery.
	var execID string
	err = database.QueryRowContext(context.Background(),
		`INSERT INTO workflow_executions (workflow_name, workflow_version, status, started_at)
		 VALUES ($1, $2, 'running', NOW()) RETURNING id`,
		"test-checkpoint", version,
	).Scan(&execID)
	if err != nil {
		t.Fatalf("inserting execution: %v", err)
	}

	// Mark step-one as already completed with known output.
	outputJSON, _ := json.Marshal(map[string]any{"call": 99})
	_, err = database.ExecContext(context.Background(),
		`INSERT INTO step_executions (execution_id, step_name, status, output, completed_at)
		 VALUES ($1, 'step-one', 'completed', $2, NOW())`,
		execID, outputJSON,
	)
	if err != nil {
		t.Fatalf("inserting step: %v", err)
	}

	// Reset call count and re-execute. The engine should skip step-one and only call step-two.
	callCount = 0
	result2, err := eng.resumeExecution(context.Background(), execID, "test-checkpoint", version, nil)
	if err != nil {
		t.Fatalf("resumeExecution() error: %v", err)
	}
	if result2.Status != "completed" {
		t.Fatalf("resumed run status = %q, want %q (error: %s)", result2.Status, "completed", result2.Error)
	}
	if callCount != 1 {
		t.Errorf("resumed run call count = %d, want 1 (should skip step-one)", callCount)
	}
}

func TestEngine_RetryPolicy(t *testing.T) {
	database := setupTestDB(t)

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 3 {
			w.WriteHeader(500)
			w.Write([]byte("not ready"))
			return
		}
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]any{"success": true})
	}))
	defer server.Close()

	wfYAML := []byte(`name: test-retry
description: Test retry policy
steps:
  - name: flaky-step
    action: http/request
    retry:
      max_attempts: 3
      backoff: fixed
    params:
      url: "` + server.URL + `"
`)
	version := applyWorkflow(t, database, wfYAML)

	eng, err := New(database)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	result, err := eng.Execute(context.Background(), "test-retry", version, nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if result.Status != "completed" {
		t.Errorf("status = %q, want %q (error: %s)", result.Status, "completed", result.Error)
	}
	if callCount != 3 {
		t.Errorf("call count = %d, want 3 (2 failures + 1 success)", callCount)
	}
}

func TestEngine_Timeout(t *testing.T) {
	database := setupTestDB(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second) // longer than timeout
		w.WriteHeader(200)
	}))
	defer server.Close()

	wfYAML := []byte(`name: test-timeout
description: Test step timeout
steps:
  - name: slow-step
    action: http/request
    timeout: "500ms"
    params:
      url: "` + server.URL + `"
`)
	version := applyWorkflow(t, database, wfYAML)

	eng, err := New(database)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	result, err := eng.Execute(context.Background(), "test-timeout", version, nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if result.Status != "failed" {
		t.Errorf("status = %q, want %q", result.Status, "failed")
	}
	if result.Steps["slow-step"].Status != "failed" {
		t.Errorf("slow-step status = %q, want %q", result.Steps["slow-step"].Status, "failed")
	}
}

func TestCheckOutputSize_UnderLimit(t *testing.T) {
	output := map[string]any{
		"status": 200,
		"body":   "small payload",
	}
	err := checkOutputSize(output, 1024)
	if err != nil {
		t.Errorf("checkOutputSize() unexpected error for small output: %v", err)
	}
}

func TestCheckOutputSize_ExactlyAtLimit(t *testing.T) {
	// Build output whose JSON serialisation is exactly at the limit.
	output := map[string]any{"k": "v"}
	data, _ := json.Marshal(output)
	limit := len(data)

	err := checkOutputSize(output, limit)
	if err != nil {
		t.Errorf("checkOutputSize() unexpected error when output equals limit: %v", err)
	}
}

func TestCheckOutputSize_OverLimit(t *testing.T) {
	// Create output that exceeds a small limit.
	output := map[string]any{
		"large": strings.Repeat("x", 2048),
	}
	limit := 64

	err := checkOutputSize(output, limit)
	if err == nil {
		t.Fatal("checkOutputSize() expected error for oversized output, got nil")
	}

	// Verify the error message contains the helpful guidance.
	if !strings.Contains(err.Error(), "S3 connector") {
		t.Errorf("error message should mention S3 connector, got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("%d byte limit", limit)) {
		t.Errorf("error message should mention the byte limit, got: %s", err.Error())
	}
}

func TestCheckOutputSize_NilOutput(t *testing.T) {
	// nil output marshals to "null" (4 bytes), should pass any reasonable limit.
	err := checkOutputSize(nil, 1024)
	if err != nil {
		t.Errorf("checkOutputSize() unexpected error for nil output: %v", err)
	}
}

func TestCheckOutputSize_EmptyMap(t *testing.T) {
	err := checkOutputSize(map[string]any{}, 1024)
	if err != nil {
		t.Errorf("checkOutputSize() unexpected error for empty map: %v", err)
	}
}
