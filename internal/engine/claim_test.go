package engine

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/dvflw/mantle/internal/db"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("mantle_test"),
		postgres.WithUsername("mantle"),
		postgres.WithPassword("mantle"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Skipf("Could not start Postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	})
	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("Failed to get connection string: %v", err)
	}

	database, err := db.Open(connStr)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := db.Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	return database
}

func createTestExecution(t *testing.T, database *sql.DB) string {
	t.Helper()
	var id string
	err := database.QueryRow(`
		INSERT INTO workflow_executions (workflow_name, workflow_version, status)
		VALUES ('test-workflow', 1, 'running')
		RETURNING id
	`).Scan(&id)
	if err != nil {
		t.Fatalf("createTestExecution: %v", err)
	}
	return id
}

func insertPendingStep(t *testing.T, database *sql.DB, execID, stepName string, attempt int) string {
	t.Helper()
	var id string
	err := database.QueryRow(`
		INSERT INTO step_executions (execution_id, step_name, attempt, status)
		VALUES ($1, $2, $3, 'pending')
		RETURNING id
	`, execID, stepName, attempt).Scan(&id)
	if err != nil {
		t.Fatalf("insertPendingStep: %v", err)
	}
	return id
}

func resetStepToPending(t *testing.T, database *sql.DB, stepID string) {
	t.Helper()
	_, err := database.Exec(`
		UPDATE step_executions
		SET status = 'pending',
		    claimed_by = NULL,
		    lease_expires_at = NULL,
		    started_at = NULL,
		    updated_at = NOW()
		WHERE id = $1
	`, stepID)
	if err != nil {
		t.Fatalf("resetStepToPending: %v", err)
	}
}

func TestClaimStep_ClaimsPendingStep(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	execID := createTestExecution(t, database)
	stepID := insertPendingStep(t, database, execID, "step-1", 1)

	claimer := &Claimer{
		DB:            database,
		NodeID:        "node-1",
		LeaseDuration: 30 * time.Second,
	}

	claim, err := claimer.ClaimStep(ctx, execID)
	if err != nil {
		t.Fatalf("ClaimStep() error = %v", err)
	}
	if claim == nil {
		t.Fatal("ClaimStep() returned nil, expected a claim")
	}
	if claim.ID != stepID {
		t.Errorf("claim.ID = %s, want %s", claim.ID, stepID)
	}
	if claim.StepName != "step-1" {
		t.Errorf("claim.StepName = %s, want step-1", claim.StepName)
	}
	if claim.Attempt != 1 {
		t.Errorf("claim.Attempt = %d, want 1", claim.Attempt)
	}
	if claim.ClaimedBy != "node-1" {
		t.Errorf("claim.ClaimedBy = %s, want node-1", claim.ClaimedBy)
	}

	// Verify database state
	var status, claimedBy string
	var leaseExpiresAt, startedAt sql.NullTime
	err = database.QueryRow(`
		SELECT status, claimed_by, lease_expires_at, started_at
		FROM step_executions WHERE id = $1
	`, stepID).Scan(&status, &claimedBy, &leaseExpiresAt, &startedAt)
	if err != nil {
		t.Fatalf("query step: %v", err)
	}
	if status != "running" {
		t.Errorf("status = %s, want running", status)
	}
	if claimedBy != "node-1" {
		t.Errorf("claimed_by = %s, want node-1", claimedBy)
	}
	if !leaseExpiresAt.Valid {
		t.Error("lease_expires_at is NULL, expected a value")
	}
	if !startedAt.Valid {
		t.Error("started_at is NULL, expected a value")
	}
}

func TestClaimStep_SkipLocked_TwoClaimers(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	execID := createTestExecution(t, database)
	insertPendingStep(t, database, execID, "only-step", 1)

	claimer1 := &Claimer{DB: database, NodeID: "node-1", LeaseDuration: 30 * time.Second}
	claimer2 := &Claimer{DB: database, NodeID: "node-2", LeaseDuration: 30 * time.Second}

	// First claimer gets the step
	claim1, err := claimer1.ClaimStep(ctx, execID)
	if err != nil {
		t.Fatalf("claimer1.ClaimStep() error = %v", err)
	}
	if claim1 == nil {
		t.Fatal("claimer1 got nil claim")
	}

	// Second claimer finds nothing (step is already running/claimed)
	claim2, err := claimer2.ClaimStep(ctx, execID)
	if err != nil {
		t.Fatalf("claimer2.ClaimStep() error = %v", err)
	}
	if claim2 != nil {
		t.Error("claimer2 should get nil claim, but got a step")
	}
}

func TestClaimAnyStep_ClaimsAcrossExecutions(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	exec1 := createTestExecution(t, database)
	exec2 := createTestExecution(t, database)
	insertPendingStep(t, database, exec1, "step-a", 1)
	insertPendingStep(t, database, exec2, "step-b", 1)

	claimer := &Claimer{DB: database, NodeID: "node-1", LeaseDuration: 30 * time.Second}

	claim1, execID1, err := claimer.ClaimAnyStep(ctx)
	if err != nil {
		t.Fatalf("ClaimAnyStep() #1 error = %v", err)
	}
	if claim1 == nil {
		t.Fatal("ClaimAnyStep() #1 returned nil")
	}
	if execID1 == "" {
		t.Error("ClaimAnyStep() #1 returned empty executionID")
	}

	claim2, execID2, err := claimer.ClaimAnyStep(ctx)
	if err != nil {
		t.Fatalf("ClaimAnyStep() #2 error = %v", err)
	}
	if claim2 == nil {
		t.Fatal("ClaimAnyStep() #2 returned nil")
	}
	if execID2 == "" {
		t.Error("ClaimAnyStep() #2 returned empty executionID")
	}

	// Two different steps were claimed
	if claim1.ID == claim2.ID {
		t.Error("both claims returned the same step ID")
	}

	// No more work
	claim3, _, err := claimer.ClaimAnyStep(ctx)
	if err != nil {
		t.Fatalf("ClaimAnyStep() #3 error = %v", err)
	}
	if claim3 != nil {
		t.Error("ClaimAnyStep() #3 should return nil, no more pending steps")
	}
}

func TestCompleteStep_Success(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	execID := createTestExecution(t, database)
	insertPendingStep(t, database, execID, "step-1", 1)

	claimer := &Claimer{DB: database, NodeID: "node-1", LeaseDuration: 30 * time.Second}

	claim, err := claimer.ClaimStep(ctx, execID)
	if err != nil {
		t.Fatalf("ClaimStep() error = %v", err)
	}

	ok, err := claimer.CompleteStep(ctx, claim.ID, []byte(`{"result":"done"}`), nil)
	if err != nil {
		t.Fatalf("CompleteStep() error = %v", err)
	}
	if !ok {
		t.Error("CompleteStep() returned false, expected true")
	}

	var status string
	var completedAt sql.NullTime
	err = database.QueryRow(`
		SELECT status, completed_at FROM step_executions WHERE id = $1
	`, claim.ID).Scan(&status, &completedAt)
	if err != nil {
		t.Fatalf("query step: %v", err)
	}
	if status != "completed" {
		t.Errorf("status = %s, want completed", status)
	}
	if !completedAt.Valid {
		t.Error("completed_at is NULL, expected a value")
	}
}

func TestCompleteStep_FencingRejectsStaleWorker(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	execID := createTestExecution(t, database)
	insertPendingStep(t, database, execID, "step-1", 1)

	claimer1 := &Claimer{DB: database, NodeID: "node-1", LeaseDuration: 30 * time.Second}
	claimer2 := &Claimer{DB: database, NodeID: "node-2", LeaseDuration: 30 * time.Second}

	claim, err := claimer1.ClaimStep(ctx, execID)
	if err != nil {
		t.Fatalf("ClaimStep() error = %v", err)
	}

	// Simulate reaper clearing the claim and node-2 reclaiming
	resetStepToPending(t, database, claim.ID)
	claim2, err := claimer2.ClaimStep(ctx, execID)
	if err != nil {
		t.Fatalf("claimer2.ClaimStep() error = %v", err)
	}
	if claim2 == nil {
		t.Fatal("claimer2 should have reclaimed the step")
	}

	// Stale node-1 tries to complete -- fencing rejects it
	ok, err := claimer1.CompleteStep(ctx, claim.ID, []byte(`{"stale":"result"}`), errors.New("should not matter"))
	if err != nil {
		t.Fatalf("CompleteStep() error = %v", err)
	}
	if ok {
		t.Error("CompleteStep() returned true for stale worker, expected false")
	}

	// Rightful owner can complete
	ok, err = claimer2.CompleteStep(ctx, claim2.ID, []byte(`{"good":"result"}`), nil)
	if err != nil {
		t.Fatalf("CompleteStep() claimer2 error = %v", err)
	}
	if !ok {
		t.Error("CompleteStep() claimer2 returned false, expected true")
	}
}

func TestRenewLease_ExtendsExpiry(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	execID := createTestExecution(t, database)
	insertPendingStep(t, database, execID, "step-1", 1)

	claimer := &Claimer{DB: database, NodeID: "node-1", LeaseDuration: 30 * time.Second}

	claim, err := claimer.ClaimStep(ctx, execID)
	if err != nil {
		t.Fatalf("ClaimStep() error = %v", err)
	}

	// Read original lease
	var originalLease time.Time
	err = database.QueryRow(`
		SELECT lease_expires_at FROM step_executions WHERE id = $1
	`, claim.ID).Scan(&originalLease)
	if err != nil {
		t.Fatalf("query lease: %v", err)
	}

	// Renew the lease
	ok, err := claimer.RenewLease(ctx, claim.ID)
	if err != nil {
		t.Fatalf("RenewLease() error = %v", err)
	}
	if !ok {
		t.Error("RenewLease() returned false, expected true")
	}

	// Verify lease was extended
	var newLease time.Time
	err = database.QueryRow(`
		SELECT lease_expires_at FROM step_executions WHERE id = $1
	`, claim.ID).Scan(&newLease)
	if err != nil {
		t.Fatalf("query new lease: %v", err)
	}

	if !newLease.After(originalLease) && !newLease.Equal(originalLease) {
		t.Errorf("new lease %v should be >= original lease %v", newLease, originalLease)
	}
}

func TestRenewLease_ReturnsFalseWhenReclaimed(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	execID := createTestExecution(t, database)
	insertPendingStep(t, database, execID, "step-1", 1)

	claimer1 := &Claimer{DB: database, NodeID: "node-1", LeaseDuration: 30 * time.Second}
	claimer2 := &Claimer{DB: database, NodeID: "node-2", LeaseDuration: 30 * time.Second}

	claim, err := claimer1.ClaimStep(ctx, execID)
	if err != nil {
		t.Fatalf("ClaimStep() error = %v", err)
	}

	// Simulate reaper reclaiming for node-2
	resetStepToPending(t, database, claim.ID)
	_, err = claimer2.ClaimStep(ctx, execID)
	if err != nil {
		t.Fatalf("claimer2.ClaimStep() error = %v", err)
	}

	// Original node tries to renew -- should fail
	ok, err := claimer1.RenewLease(ctx, claim.ID)
	if err != nil {
		t.Fatalf("RenewLease() error = %v", err)
	}
	if ok {
		t.Error("RenewLease() returned true for stale worker, expected false")
	}
}
