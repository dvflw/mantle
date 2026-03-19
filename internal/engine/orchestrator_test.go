package engine

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"
	"time"
)

func newTestOrchestrator(database *sql.DB, nodeID string) *Orchestrator {
	return &Orchestrator{
		DB:            database,
		NodeID:        nodeID,
		LeaseDuration: 30 * time.Second,
		PollInterval:  1 * time.Second,
		Logger:        slog.Default(),
	}
}

func TestOrchestrator_ClaimExecution(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()
	execID := createTestExecution(t, database)

	orch1 := newTestOrchestrator(database, "node-1")
	orch2 := newTestOrchestrator(database, "node-2")

	// First claim should succeed.
	claimed, err := orch1.ClaimExecution(ctx, execID)
	if err != nil {
		t.Fatalf("ClaimExecution() error = %v", err)
	}
	if !claimed {
		t.Fatal("ClaimExecution() first claim should succeed")
	}

	// Second claim by a different node should fail.
	claimed, err = orch2.ClaimExecution(ctx, execID)
	if err != nil {
		t.Fatalf("ClaimExecution() error = %v", err)
	}
	if claimed {
		t.Fatal("ClaimExecution() second claim should fail")
	}
}

func TestOrchestrator_RenewExecutionLease(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()
	execID := createTestExecution(t, database)

	orch := newTestOrchestrator(database, "node-1")

	claimed, err := orch.ClaimExecution(ctx, execID)
	if err != nil {
		t.Fatalf("ClaimExecution() error = %v", err)
	}
	if !claimed {
		t.Fatal("ClaimExecution() should succeed")
	}

	// Read the initial lease expiry.
	var initialExpiry time.Time
	err = database.QueryRow(
		`SELECT lease_expires_at FROM execution_claims WHERE execution_id = $1`,
		execID,
	).Scan(&initialExpiry)
	if err != nil {
		t.Fatalf("QueryRow error = %v", err)
	}

	// Small sleep to ensure time difference.
	time.Sleep(10 * time.Millisecond)

	// Renew the lease.
	if err := orch.RenewExecutionLease(ctx, execID); err != nil {
		t.Fatalf("RenewExecutionLease() error = %v", err)
	}

	// Verify lease was extended.
	var newExpiry time.Time
	err = database.QueryRow(
		`SELECT lease_expires_at FROM execution_claims WHERE execution_id = $1`,
		execID,
	).Scan(&newExpiry)
	if err != nil {
		t.Fatalf("QueryRow error = %v", err)
	}

	if !newExpiry.After(initialExpiry) {
		t.Errorf("RenewExecutionLease() new expiry %v should be after initial %v", newExpiry, initialExpiry)
	}
}

func TestOrchestrator_ReleaseExecution(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()
	execID := createTestExecution(t, database)

	orch1 := newTestOrchestrator(database, "node-1")
	orch2 := newTestOrchestrator(database, "node-2")

	// Claim and release.
	claimed, err := orch1.ClaimExecution(ctx, execID)
	if err != nil {
		t.Fatalf("ClaimExecution() error = %v", err)
	}
	if !claimed {
		t.Fatal("ClaimExecution() should succeed")
	}

	if err := orch1.ReleaseExecution(ctx, execID); err != nil {
		t.Fatalf("ReleaseExecution() error = %v", err)
	}

	// Another node should now be able to claim it.
	claimed, err = orch2.ClaimExecution(ctx, execID)
	if err != nil {
		t.Fatalf("ClaimExecution() after release error = %v", err)
	}
	if !claimed {
		t.Fatal("ClaimExecution() should succeed after release")
	}
}

func TestOrchestrator_CreatePendingSteps(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()
	execID := createTestExecution(t, database)

	orch := newTestOrchestrator(database, "node-1")

	stepNames := []string{"fetch-data", "transform", "notify"}
	maxAttempts := map[string]int{
		"fetch-data": 3,
		"transform":  2,
		"notify":     1,
	}

	if err := orch.CreatePendingSteps(ctx, execID, stepNames, maxAttempts); err != nil {
		t.Fatalf("CreatePendingSteps() error = %v", err)
	}

	// Verify the rows were created.
	var count int
	err := database.QueryRow(
		`SELECT COUNT(*) FROM step_executions WHERE execution_id = $1 AND status = 'pending'`,
		execID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("QueryRow error = %v", err)
	}
	if count != 3 {
		t.Errorf("CreatePendingSteps() created %d rows, want 3", count)
	}

	// Verify max_attempts values.
	var ma int
	err = database.QueryRow(
		`SELECT max_attempts FROM step_executions WHERE execution_id = $1 AND step_name = 'fetch-data'`,
		execID,
	).Scan(&ma)
	if err != nil {
		t.Fatalf("QueryRow error = %v", err)
	}
	if ma != 3 {
		t.Errorf("max_attempts for fetch-data = %d, want 3", ma)
	}

	// Verify idempotency: calling again should not error.
	if err := orch.CreatePendingSteps(ctx, execID, stepNames, maxAttempts); err != nil {
		t.Fatalf("CreatePendingSteps() second call error = %v", err)
	}

	err = database.QueryRow(
		`SELECT COUNT(*) FROM step_executions WHERE execution_id = $1 AND status = 'pending'`,
		execID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("QueryRow error = %v", err)
	}
	if count != 3 {
		t.Errorf("CreatePendingSteps() idempotent call resulted in %d rows, want 3", count)
	}
}

func TestOrchestrator_GetStepStatuses(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()
	execID := createTestExecution(t, database)

	// Insert two attempts for the same step; the latest attempt should be returned.
	insertPendingStep(t, database, execID, "fetch-data", 1)
	// Update attempt 1 to failed.
	_, err := database.Exec(
		`UPDATE step_executions SET status = 'failed', error = 'timeout'
		 WHERE execution_id = $1 AND step_name = 'fetch-data' AND attempt = 1`,
		execID,
	)
	if err != nil {
		t.Fatalf("Update error = %v", err)
	}

	// Insert attempt 2 as running.
	insertPendingStep(t, database, execID, "fetch-data", 2)
	_, err = database.Exec(
		`UPDATE step_executions SET status = 'running'
		 WHERE execution_id = $1 AND step_name = 'fetch-data' AND attempt = 2`,
		execID,
	)
	if err != nil {
		t.Fatalf("Update error = %v", err)
	}

	// Insert another step.
	insertPendingStep(t, database, execID, "notify", 1)

	orch := newTestOrchestrator(database, "node-1")
	statuses, err := orch.GetStepStatuses(ctx, execID)
	if err != nil {
		t.Fatalf("GetStepStatuses() error = %v", err)
	}

	if len(statuses) != 2 {
		t.Fatalf("GetStepStatuses() returned %d statuses, want 2", len(statuses))
	}

	// fetch-data should show the latest attempt (attempt 2, running).
	fd, ok := statuses["fetch-data"]
	if !ok {
		t.Fatal("GetStepStatuses() missing fetch-data")
	}
	if fd.Status != "running" {
		t.Errorf("fetch-data status = %q, want %q", fd.Status, "running")
	}
	if fd.Attempt != 2 {
		t.Errorf("fetch-data attempt = %d, want 2", fd.Attempt)
	}

	// notify should be pending.
	notify, ok := statuses["notify"]
	if !ok {
		t.Fatal("GetStepStatuses() missing notify")
	}
	if notify.Status != "pending" {
		t.Errorf("notify status = %q, want %q", notify.Status, "pending")
	}
}

func TestOrchestrator_CreateRetryStep(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()
	execID := createTestExecution(t, database)

	// Insert initial attempt.
	insertPendingStep(t, database, execID, "fetch-data", 1)

	orch := newTestOrchestrator(database, "node-1")

	// Create retry attempt.
	if err := orch.CreateRetryStep(ctx, execID, "fetch-data", 2, 3); err != nil {
		t.Fatalf("CreateRetryStep() error = %v", err)
	}

	// Verify the new attempt exists.
	var status string
	var attempt, maxAttempts int
	err := database.QueryRow(
		`SELECT status, attempt, max_attempts FROM step_executions
		 WHERE execution_id = $1 AND step_name = 'fetch-data' AND attempt = 2`,
		execID,
	).Scan(&status, &attempt, &maxAttempts)
	if err != nil {
		t.Fatalf("QueryRow error = %v", err)
	}
	if status != "pending" {
		t.Errorf("retry step status = %q, want %q", status, "pending")
	}
	if attempt != 2 {
		t.Errorf("retry step attempt = %d, want 2", attempt)
	}
	if maxAttempts != 3 {
		t.Errorf("retry step max_attempts = %d, want 3", maxAttempts)
	}
}

func TestOrchestrator_CancelPendingSteps(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()
	execID := createTestExecution(t, database)

	// Create some pending steps.
	insertPendingStep(t, database, execID, "step-a", 1)
	insertPendingStep(t, database, execID, "step-b", 1)
	insertPendingStep(t, database, execID, "step-c", 1)

	orch := newTestOrchestrator(database, "node-1")

	// Cancel only step-a and step-b.
	if err := orch.CancelPendingSteps(ctx, execID, []string{"step-a", "step-b"}); err != nil {
		t.Fatalf("CancelPendingSteps() error = %v", err)
	}

	// Verify step-a and step-b are cancelled.
	for _, name := range []string{"step-a", "step-b"} {
		var status string
		err := database.QueryRow(
			`SELECT status FROM step_executions WHERE execution_id = $1 AND step_name = $2`,
			execID, name,
		).Scan(&status)
		if err != nil {
			t.Fatalf("QueryRow error for %s: %v", name, err)
		}
		if status != "cancelled" {
			t.Errorf("%s status = %q, want %q", name, status, "cancelled")
		}
	}

	// step-c should still be pending.
	var status string
	err := database.QueryRow(
		`SELECT status FROM step_executions WHERE execution_id = $1 AND step_name = 'step-c'`,
		execID,
	).Scan(&status)
	if err != nil {
		t.Fatalf("QueryRow error for step-c: %v", err)
	}
	if status != "pending" {
		t.Errorf("step-c status = %q, want %q", status, "pending")
	}
}
