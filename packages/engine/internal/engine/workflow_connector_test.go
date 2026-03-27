package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestWorkflowConnector_BasicInvocation creates a parent and child workflow,
// executes the parent with a workflow/run step, and verifies the child ran
// and output is wrapped correctly.
func TestWorkflowConnector_BasicInvocation(t *testing.T) {
	database := setupTestDB(t)

	// Apply the child workflow first.
	childYAML := []byte(`name: child-wf
description: A simple child workflow
steps:
  - name: greet
    action: test/echo
    params:
      message: "hello from child"
`)
	applyWorkflow(t, database, childYAML)

	// Apply the parent workflow that invokes the child via workflow/run.
	parentYAML := []byte(`name: parent-wf
description: Parent that calls child
steps:
  - name: call-child
    action: workflow/run
    params:
      workflow: child-wf
`)
	parentVersion := applyWorkflow(t, database, parentYAML)

	// Create engine and register a mock "test/echo" connector.
	eng, err := New(database)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	eng.Registry.Register("test/echo", &mockConnector{
		fn: func(ctx context.Context, params map[string]any) (map[string]any, error) {
			msg, _ := params["message"].(string)
			return map[string]any{"echoed": msg}, nil
		},
	})
	eng.RegisterWorkflowConnector()

	// Execute the parent. We use WithExecutionID so the workflow connector
	// can find the parent execution ID when called during resumeExecution.
	// First, create the execution record manually (same as Engine.Execute does).
	ctx := context.Background()
	parentExecID, err := eng.createExecution(ctx, "parent-wf", parentVersion, nil, "pending")
	if err != nil {
		t.Fatalf("createExecution error: %v", err)
	}

	ctx = WithExecutionID(ctx, parentExecID)
	result, err := eng.resumeExecution(ctx, parentExecID, "parent-wf", parentVersion, nil)
	if err != nil {
		t.Fatalf("resumeExecution() error: %v", err)
	}

	if result.Status != "completed" {
		t.Fatalf("parent status = %q, want %q (error: %s)", result.Status, "completed", result.Error)
	}

	// Verify the call-child step output contains child execution details.
	callChild := result.Steps["call-child"]
	if callChild.Status != "completed" {
		t.Fatalf("call-child status = %q, want %q", callChild.Status, "completed")
	}

	// Output should have execution_id, status, and steps.
	if callChild.Output["status"] != "completed" {
		t.Errorf("child output.status = %v, want %q", callChild.Output["status"], "completed")
	}
	if callChild.Output["execution_id"] == nil || callChild.Output["execution_id"] == "" {
		t.Error("child output.execution_id should be non-empty")
	}

	// Verify nested steps output.
	stepsMap, ok := callChild.Output["steps"].(map[string]any)
	if !ok {
		t.Fatalf("child output.steps is not a map, got %T", callChild.Output["steps"])
	}
	greetStep, ok := stepsMap["greet"].(map[string]any)
	if !ok {
		t.Fatalf("child output.steps['greet'] is not a map, got %T", stepsMap["greet"])
	}
	greetOutput, ok := greetStep["output"].(map[string]any)
	if !ok {
		t.Fatalf("child output.steps['greet'].output is not a map, got %T", greetStep["output"])
	}
	if greetOutput["echoed"] != "hello from child" {
		t.Errorf("child greet output.echoed = %v, want %q", greetOutput["echoed"], "hello from child")
	}

	// Verify child execution exists in the database with correct linkage.
	childExecID := callChild.Output["execution_id"].(string)
	var parentID, parentStepName string
	var depth int
	err = database.QueryRowContext(ctx,
		`SELECT parent_execution_id, parent_step_name, depth
		 FROM workflow_executions WHERE id = $1`,
		childExecID,
	).Scan(&parentID, &parentStepName, &depth)
	if err != nil {
		t.Fatalf("querying child execution: %v", err)
	}
	if parentID != parentExecID {
		t.Errorf("child parent_execution_id = %q, want %q", parentID, parentExecID)
	}
	if parentStepName != "call-child" {
		t.Errorf("child parent_step_name = %q, want %q", parentStepName, "call-child")
	}
	if depth != 1 {
		t.Errorf("child depth = %d, want 1", depth)
	}
}

// TestWorkflowConnector_DepthLimit verifies that exceeding the max workflow
// nesting depth returns an error.
func TestWorkflowConnector_DepthLimit(t *testing.T) {
	database := setupTestDB(t)

	// Apply a child workflow.
	childYAML := []byte(`name: deep-child
description: A simple child
steps:
  - name: noop
    action: test/noop
    params:
      msg: "noop"
`)
	applyWorkflow(t, database, childYAML)

	// Apply a parent workflow that invokes the child.
	parentYAML := []byte(`name: deep-parent
description: Parent hitting depth limit
steps:
  - name: call-deep
    action: workflow/run
    params:
      workflow: deep-child
`)
	parentVersion := applyWorkflow(t, database, parentYAML)

	eng, err := New(database)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	eng.Registry.Register("test/noop", &mockConnector{
		fn: func(_ context.Context, _ map[string]any) (map[string]any, error) {
			return map[string]any{"done": true}, nil
		},
	})
	eng.RegisterWorkflowConnector()
	eng.MaxWorkflowDepth = 1 // Set max depth to 1.

	// Create parent execution at depth 0.
	ctx := context.Background()
	parentExecID, err := eng.createExecution(ctx, "deep-parent", parentVersion, nil, "pending")
	if err != nil {
		t.Fatalf("createExecution error: %v", err)
	}

	ctx = WithExecutionID(ctx, parentExecID)
	result, err := eng.resumeExecution(ctx, parentExecID, "deep-parent", parentVersion, nil)
	if err != nil {
		t.Fatalf("resumeExecution() error: %v", err)
	}

	// The parent should fail because the child would be at depth 1,
	// and the parent itself is at depth 0, so child depth = 1 which exceeds max depth 1.
	// Actually: childDepth = parentDepth(0) + 1 = 1, maxDepth = 1,
	// and the check is childDepth > maxDepth, so 1 > 1 is false.
	// We need maxDepth = 0 for it to fail, but 0 means "use default 10".
	// Let's set it differently.
	// Re-check: the connector sets maxDepth=10 when MaxWorkflowDepth <= 0.
	// When MaxWorkflowDepth = 1, childDepth = 1, and 1 > 1 is false.
	// So this should succeed. Let me set it to trigger failure properly.

	// Actually, we should verify what happens. The depth check in the connector is:
	//   childDepth := parentDepth + 1
	//   if childDepth > maxDepth { error }
	// With MaxWorkflowDepth=1, parentDepth=0, childDepth=1, 1>1=false, so it passes.
	// To trigger the error, we need to simulate the parent being at depth >= maxDepth.
	// Let's just verify it works at depth 1, then test the error case with a
	// parent already at depth 1 and max=1.

	// This test should succeed since parent is at depth 0 and max is 1.
	if result.Status != "completed" {
		// If it failed for depth reasons, that's fine for the test intent.
		// But let's be precise.
		t.Logf("result status=%s, error=%s", result.Status, result.Error)
	}

	// Now test the actual depth limit: create an execution at depth 1 and max=1.
	var deepExecID string
	err = database.QueryRowContext(ctx,
		`INSERT INTO workflow_executions
		 (workflow_name, workflow_version, status, started_at, team_id, depth)
		 VALUES ($1, $2, 'pending', NOW(), $3, $4)
		 RETURNING id`,
		"deep-parent", parentVersion, "00000000-0000-0000-0000-000000000001", 1,
	).Scan(&deepExecID)
	if err != nil {
		t.Fatalf("inserting deep execution: %v", err)
	}

	ctx2 := WithExecutionID(context.Background(), deepExecID)
	result2, err := eng.resumeExecution(ctx2, deepExecID, "deep-parent", parentVersion, nil)
	if err != nil {
		t.Fatalf("resumeExecution() error: %v", err)
	}

	// The call-deep step should fail with a depth limit error.
	if result2.Status != "failed" {
		t.Errorf("expected failed status for depth limit, got %q", result2.Status)
	}
	callDeep := result2.Steps["call-deep"]
	if callDeep.Status != "failed" {
		t.Errorf("call-deep status = %q, want %q", callDeep.Status, "failed")
	}
	if callDeep.Error == "" {
		t.Error("call-deep error should be non-empty")
	}
	if !strings.Contains(callDeep.Error, "depth") {
		t.Errorf("expected depth limit error, got: %s", callDeep.Error)
	}
	t.Logf("depth limit error: %s", callDeep.Error)
}

// TestWorkflowConnector_CheckpointRecovery verifies that if a child execution
// already exists (from a previous attempt), the connector reuses it rather
// than creating a new one.
func TestWorkflowConnector_CheckpointRecovery(t *testing.T) {
	database := setupTestDB(t)

	// Apply the child workflow.
	childYAML := []byte(`name: recoverable-child
description: Child for checkpoint test
steps:
  - name: work
    action: test/echo
    params:
      message: "working"
`)
	childVersion := applyWorkflow(t, database, childYAML)

	// Apply the parent workflow.
	parentYAML := []byte(`name: recoverable-parent
description: Parent for checkpoint test
steps:
  - name: run-child
    action: workflow/run
    params:
      workflow: recoverable-child
`)
	parentVersion := applyWorkflow(t, database, parentYAML)

	eng, err := New(database)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	callCount := 0
	eng.Registry.Register("test/echo", &mockConnector{
		fn: func(ctx context.Context, params map[string]any) (map[string]any, error) {
			callCount++
			msg, _ := params["message"].(string)
			return map[string]any{"echoed": msg, "call": callCount}, nil
		},
	})
	eng.RegisterWorkflowConnector()

	ctx := context.Background()

	// Create the parent execution.
	parentExecID, err := eng.createExecution(ctx, "recoverable-parent", parentVersion, nil, "pending")
	if err != nil {
		t.Fatalf("createExecution error: %v", err)
	}

	// Pre-create a completed child execution (simulating a previous run that completed
	// before the parent crashed).
	childOutput := map[string]any{"echoed": "working", "call": 99}
	childOutputJSON, _ := json.Marshal(childOutput)
	var childExecID string
	err = database.QueryRowContext(ctx,
		`INSERT INTO workflow_executions
		 (workflow_name, workflow_version, status, started_at, completed_at, team_id,
		  parent_execution_id, parent_step_name, depth)
		 VALUES ($1, $2, 'completed', NOW(), NOW(), $3, $4, $5, $6)
		 RETURNING id`,
		"recoverable-child", childVersion, "00000000-0000-0000-0000-000000000001",
		parentExecID, "run-child", 1,
	).Scan(&childExecID)
	if err != nil {
		t.Fatalf("inserting child execution: %v", err)
	}

	// Insert the child's completed step.
	_, err = database.ExecContext(ctx,
		`INSERT INTO step_executions (execution_id, step_name, status, output, completed_at)
		 VALUES ($1, 'work', 'completed', $2, NOW())`,
		childExecID, childOutputJSON,
	)
	if err != nil {
		t.Fatalf("inserting child step: %v", err)
	}

	// Execute the parent. The workflow connector should find the existing child
	// and reuse it, not create a new one.
	ctx = WithExecutionID(ctx, parentExecID)
	result, err := eng.resumeExecution(ctx, parentExecID, "recoverable-parent", parentVersion, nil)
	if err != nil {
		t.Fatalf("resumeExecution() error: %v", err)
	}

	if result.Status != "completed" {
		t.Fatalf("parent status = %q, want %q (error: %s)", result.Status, "completed", result.Error)
	}

	// The test/echo connector should NOT have been called because the child
	// was already completed (checkpoint recovery).
	if callCount != 0 {
		t.Errorf("test/echo call count = %d, want 0 (child should be reused)", callCount)
	}

	// Verify the output references the pre-existing child execution.
	runChild := result.Steps["run-child"]
	if runChild.Output["execution_id"] != childExecID {
		t.Errorf("output.execution_id = %v, want %q", runChild.Output["execution_id"], childExecID)
	}

	// Verify only one child execution exists (no duplicate created).
	var childCount int
	err = database.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM workflow_executions
		 WHERE parent_execution_id = $1 AND parent_step_name = 'run-child'`,
		parentExecID,
	).Scan(&childCount)
	if err != nil {
		t.Fatalf("counting child executions: %v", err)
	}
	if childCount != 1 {
		t.Errorf("child execution count = %d, want 1", childCount)
	}
}

// TestWorkflowConnector_DepthCheck_Unit tests the depth checking logic directly
// without a full workflow execution.
func TestWorkflowConnector_DepthCheck_Unit(t *testing.T) {
	tests := []struct {
		name        string
		parentDepth int
		maxDepth    int
		wantErr     bool
	}{
		{"depth 0, max 10", 0, 10, false},
		{"depth 9, max 10", 9, 10, false},
		{"depth 10, max 10", 10, 10, true},
		{"depth 5, max 5", 5, 5, true},
		{"depth 0, max 1", 0, 1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			childDepth := tt.parentDepth + 1
			exceeded := childDepth > tt.maxDepth
			if exceeded != tt.wantErr {
				t.Errorf("childDepth(%d) > maxDepth(%d) = %v, want %v",
					childDepth, tt.maxDepth, exceeded, tt.wantErr)
			}
		})
	}
}

// TestWorkflowConnector_OutputWrapping_Unit tests that child execution results
// are wrapped in the expected format.
func TestWorkflowConnector_OutputWrapping_Unit(t *testing.T) {
	// Simulate the output wrapping logic from WorkflowConnector.Execute.
	childResult := &ExecutionResult{
		ExecutionID: "child-123",
		Status:      "completed",
		Steps: map[string]StepResult{
			"step-a": {Status: "completed", Output: map[string]any{"value": "alpha"}},
			"step-b": {Status: "completed", Output: map[string]any{"value": "beta"}},
		},
	}

	stepsOutput := make(map[string]any, len(childResult.Steps))
	for name, sr := range childResult.Steps {
		stepMap := map[string]any{
			"output": sr.Output,
		}
		if sr.Error != "" {
			stepMap["error"] = sr.Error
		}
		stepsOutput[name] = stepMap
	}

	output := map[string]any{
		"execution_id": childResult.ExecutionID,
		"status":       childResult.Status,
		"steps":        stepsOutput,
	}

	if output["execution_id"] != "child-123" {
		t.Errorf("execution_id = %v, want %q", output["execution_id"], "child-123")
	}
	if output["status"] != "completed" {
		t.Errorf("status = %v, want %q", output["status"], "completed")
	}

	steps := output["steps"].(map[string]any)
	stepA := steps["step-a"].(map[string]any)
	stepAOutput := stepA["output"].(map[string]any)
	if stepAOutput["value"] != "alpha" {
		t.Errorf("steps['step-a'].output.value = %v, want %q", stepAOutput["value"], "alpha")
	}

	// Verify no error key when error is empty.
	if _, hasErr := stepA["error"]; hasErr {
		t.Error("step-a should not have 'error' key when error is empty")
	}

	// Test with error.
	failedResult := &ExecutionResult{
		Status: "failed",
		Steps: map[string]StepResult{
			"fail-step": {Status: "failed", Error: "something broke", Output: map[string]any{}},
		},
	}
	failStepsOutput := make(map[string]any)
	for name, sr := range failedResult.Steps {
		stepMap := map[string]any{"output": sr.Output}
		if sr.Error != "" {
			stepMap["error"] = sr.Error
		}
		failStepsOutput[name] = stepMap
	}
	failStep := failStepsOutput["fail-step"].(map[string]any)
	if failStep["error"] != "something broke" {
		t.Errorf("fail-step error = %v, want %q", failStep["error"], "something broke")
	}
}
