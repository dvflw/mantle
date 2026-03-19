package engine

import (
	"context"
	"testing"
)

func TestToolSteps_CreateSubStep(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	execID := createTestExecution(t, database)
	parentID := insertPendingStep(t, database, execID, "parent-step")

	ts := &ToolSteps{DB: database}

	childID, err := ts.CreateSubStep(ctx, execID, parentID, "child-step-1", 3)
	if err != nil {
		t.Fatalf("CreateSubStep() error = %v", err)
	}
	if childID == "" {
		t.Fatal("CreateSubStep() returned empty ID")
	}

	// Verify the child step exists in the DB with correct parent reference.
	var gotParentID string
	var gotStatus string
	err = database.QueryRowContext(ctx, `
		SELECT parent_step_id, status FROM step_executions WHERE id = $1
	`, childID).Scan(&gotParentID, &gotStatus)
	if err != nil {
		t.Fatalf("QueryRow() error = %v", err)
	}
	if gotParentID != parentID {
		t.Errorf("parent_step_id = %s, want %s", gotParentID, parentID)
	}
	if gotStatus != "pending" {
		t.Errorf("status = %s, want pending", gotStatus)
	}
}

func TestToolSteps_CreateSubStep_Idempotent(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	execID := createTestExecution(t, database)
	parentID := insertPendingStep(t, database, execID, "parent-step")

	ts := &ToolSteps{DB: database}

	id1, err := ts.CreateSubStep(ctx, execID, parentID, "child-idem", 3)
	if err != nil {
		t.Fatalf("CreateSubStep() first call error = %v", err)
	}

	id2, err := ts.CreateSubStep(ctx, execID, parentID, "child-idem", 3)
	if err != nil {
		t.Fatalf("CreateSubStep() second call error = %v", err)
	}

	if id1 != id2 {
		t.Errorf("Idempotent call returned different IDs: %s vs %s", id1, id2)
	}

	// Verify only one row exists.
	var count int
	err = database.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM step_executions
		WHERE execution_id = $1 AND step_name = 'child-idem'
	`, execID).Scan(&count)
	if err != nil {
		t.Fatalf("QueryRow() error = %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 row, got %d", count)
	}
}

func TestToolSteps_CacheLLMResponse(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	execID := createTestExecution(t, database)
	stepID := insertPendingStep(t, database, execID, "llm-step")

	ts := &ToolSteps{DB: database}

	resp1 := map[string]any{
		"model":   "gpt-4",
		"content": "Hello, world!",
		"tokens":  float64(42),
	}
	resp2 := map[string]any{
		"model":   "gpt-4",
		"content": "Follow-up response",
		"tokens":  float64(17),
	}

	if err := ts.CacheLLMResponse(ctx, stepID, resp1); err != nil {
		t.Fatalf("CacheLLMResponse() first call error = %v", err)
	}
	if err := ts.CacheLLMResponse(ctx, stepID, resp2); err != nil {
		t.Fatalf("CacheLLMResponse() second call error = %v", err)
	}

	// Load and verify.
	responses, err := ts.LoadCachedLLMResponses(ctx, stepID)
	if err != nil {
		t.Fatalf("LoadCachedLLMResponses() error = %v", err)
	}
	if len(responses) != 2 {
		t.Fatalf("Expected 2 responses, got %d", len(responses))
	}

	// Verify order and content.
	if responses[0]["content"] != "Hello, world!" {
		t.Errorf("First response content = %v, want 'Hello, world!'", responses[0]["content"])
	}
	if responses[1]["content"] != "Follow-up response" {
		t.Errorf("Second response content = %v, want 'Follow-up response'", responses[1]["content"])
	}
	if responses[0]["tokens"] != float64(42) {
		t.Errorf("First response tokens = %v, want 42", responses[0]["tokens"])
	}
}

func TestToolSteps_LoadSubStepStatuses(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	execID := createTestExecution(t, database)
	parentID := insertPendingStep(t, database, execID, "parent-step")

	ts := &ToolSteps{DB: database}

	// Create multiple child steps.
	_, err := ts.CreateSubStep(ctx, execID, parentID, "fetch-data", 3)
	if err != nil {
		t.Fatalf("CreateSubStep(fetch-data) error = %v", err)
	}
	_, err = ts.CreateSubStep(ctx, execID, parentID, "transform", 2)
	if err != nil {
		t.Fatalf("CreateSubStep(transform) error = %v", err)
	}

	// Update one child to completed with output.
	_, err = database.ExecContext(ctx, `
		UPDATE step_executions
		SET status = 'completed', output = '{"result": "ok"}'::jsonb
		WHERE execution_id = $1 AND step_name = 'fetch-data'
	`, execID)
	if err != nil {
		t.Fatalf("Update step error = %v", err)
	}

	// Update another to failed with error.
	_, err = database.ExecContext(ctx, `
		UPDATE step_executions
		SET status = 'failed', error = 'connection timeout'
		WHERE execution_id = $1 AND step_name = 'transform'
	`, execID)
	if err != nil {
		t.Fatalf("Update step error = %v", err)
	}

	// Load and verify.
	statuses, err := ts.LoadSubStepStatuses(ctx, parentID)
	if err != nil {
		t.Fatalf("LoadSubStepStatuses() error = %v", err)
	}
	if len(statuses) != 2 {
		t.Fatalf("Expected 2 statuses, got %d", len(statuses))
	}

	fetch := statuses["fetch-data"]
	if fetch == nil {
		t.Fatal("Missing status for 'fetch-data'")
	}
	if fetch.Status != "completed" {
		t.Errorf("fetch-data status = %s, want completed", fetch.Status)
	}
	if fetch.Output["result"] != "ok" {
		t.Errorf("fetch-data output = %v, want {result: ok}", fetch.Output)
	}

	transform := statuses["transform"]
	if transform == nil {
		t.Fatal("Missing status for 'transform'")
	}
	if transform.Status != "failed" {
		t.Errorf("transform status = %s, want failed", transform.Status)
	}
	if transform.Error != "connection timeout" {
		t.Errorf("transform error = %s, want 'connection timeout'", transform.Error)
	}
}
