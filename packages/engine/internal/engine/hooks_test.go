package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	mantleCEL "github.com/dvflw/mantle/internal/cel"
	"github.com/dvflw/mantle/internal/workflow"
)

func TestExecuteHooks_NilHooksIsNoop(t *testing.T) {
	database := setupTestDB(t)

	eng, err := New(database)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	celCtx := &mantleCEL.Context{
		Steps:  make(map[string]map[string]any),
		Inputs: make(map[string]any),
	}

	// wf.Hooks is nil — should return without panic.
	wf := &workflow.Workflow{
		Name:  "test-no-hooks",
		Hooks: nil,
	}

	execID := createTestExecution(t, database)

	// Should not panic.
	eng.executeHooks(context.Background(), execID, "test-no-hooks", wf, "completed", "", "", celCtx, StepContext{})
}

func TestExecuteHooks_OnSuccessFiresOnCompletion(t *testing.T) {
	database := setupTestDB(t)

	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]any{"notified": true})
	}))
	defer server.Close()

	// Apply workflow with on_success hook.
	wfYAML := []byte(`name: test-hooks-success
description: Test on_success hook
steps:
  - name: main-step
    action: http/request
    params:
      method: GET
      url: "` + server.URL + `"
hooks:
  on_success:
    - name: notify-success
      action: http/request
      params:
        method: GET
        url: "` + server.URL + `"
`)
	version := applyWorkflow(t, database, wfYAML)

	eng, err := New(database)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	result, err := eng.Execute(context.Background(), "test-hooks-success", version, nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if result.Status != "completed" {
		t.Errorf("status = %q, want %q (error: %s)", result.Status, "completed", result.Error)
	}
	if !called {
		t.Error("on_success hook should have been called on completion")
	}
}

func TestExecuteHooks_OnFailureFiresOnFailure(t *testing.T) {
	database := setupTestDB(t)

	hookCalled := false
	hookServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hookCalled = true
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]any{"hook": "fired"})
	}))
	defer hookServer.Close()

	failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("fail"))
	}))
	defer failServer.Close()

	wfYAML := []byte(`name: test-hooks-failure
description: Test on_failure hook
steps:
  - name: fail-step
    action: http/request
    params:
      method: GET
      url: "` + failServer.URL + `"
hooks:
  on_failure:
    - name: notify-failure
      action: http/request
      params:
        method: GET
        url: "` + hookServer.URL + `"
`)
	version := applyWorkflow(t, database, wfYAML)

	eng, err := New(database)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	result, err := eng.Execute(context.Background(), "test-hooks-failure", version, nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if result.Status != "failed" {
		t.Errorf("status = %q, want %q", result.Status, "failed")
	}
	if !hookCalled {
		t.Error("on_failure hook should have been called on failure")
	}
}

func TestExecuteHooks_OnFinishAlwaysRuns(t *testing.T) {
	database := setupTestDB(t)

	finishCallCount := 0
	finishServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		finishCallCount++
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]any{"finished": true})
	}))
	defer finishServer.Close()

	successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer successServer.Close()

	failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("fail"))
	}))
	defer failServer.Close()

	// Test 1: on_finish runs on success.
	wfYAML := []byte(`name: test-hooks-finish-ok
description: Test on_finish runs on success
steps:
  - name: ok-step
    action: http/request
    params:
      method: GET
      url: "` + successServer.URL + `"
hooks:
  on_finish:
    - name: cleanup
      action: http/request
      params:
        method: GET
        url: "` + finishServer.URL + `"
`)
	version := applyWorkflow(t, database, wfYAML)

	eng, err := New(database)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	result, err := eng.Execute(context.Background(), "test-hooks-finish-ok", version, nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result.Status != "completed" {
		t.Errorf("status = %q, want %q", result.Status, "completed")
	}
	if finishCallCount != 1 {
		t.Errorf("on_finish call count = %d, want 1 (on success)", finishCallCount)
	}

	// Test 2: on_finish runs on failure too.
	finishCallCount = 0
	wfYAML2 := []byte(`name: test-hooks-finish-fail
description: Test on_finish runs on failure
steps:
  - name: fail-step
    action: http/request
    params:
      method: GET
      url: "` + failServer.URL + `"
hooks:
  on_finish:
    - name: cleanup
      action: http/request
      params:
        method: GET
        url: "` + finishServer.URL + `"
`)
	version2 := applyWorkflow(t, database, wfYAML2)

	result2, err := eng.Execute(context.Background(), "test-hooks-finish-fail", version2, nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result2.Status != "failed" {
		t.Errorf("status = %q, want %q", result2.Status, "failed")
	}
	if finishCallCount != 1 {
		t.Errorf("on_finish call count = %d, want 1 (on failure)", finishCallCount)
	}
}
