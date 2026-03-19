package engine

import (
	"context"
	"database/sql"
	"log/slog"
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
		t.Fatalf("Failed to open database: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := db.Migrate(ctx, database); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	return database
}

// createTestExecution inserts a workflow execution and returns its UUID.
func createTestExecution(t *testing.T, database *sql.DB) string {
	t.Helper()
	var id string
	err := database.QueryRow(`
		INSERT INTO workflow_executions (workflow_name, workflow_version, status)
		VALUES ('test-workflow', 1, 'running')
		RETURNING id
	`).Scan(&id)
	if err != nil {
		t.Fatalf("Failed to create test execution: %v", err)
	}
	return id
}

// insertPendingStep inserts a step execution and returns its UUID.
// The step is created with status 'running' and the given lease expiry.
func insertRunningStep(t *testing.T, database *sql.DB, executionID string, stepName string, leaseExpiresAt time.Time) string {
	t.Helper()
	var id string
	err := database.QueryRow(`
		INSERT INTO step_executions (execution_id, step_name, status, claimed_by, lease_expires_at, started_at)
		VALUES ($1, $2, 'running', 'node-1', $3, NOW())
		RETURNING id
	`, executionID, stepName, leaseExpiresAt).Scan(&id)
	if err != nil {
		t.Fatalf("Failed to insert running step: %v", err)
	}
	return id
}

func insertPendingStep(t *testing.T, database *sql.DB, executionID string, stepName string) string {
	t.Helper()
	var id string
	err := database.QueryRow(`
		INSERT INTO step_executions (execution_id, step_name, status)
		VALUES ($1, $2, 'pending')
		RETURNING id
	`, executionID, stepName).Scan(&id)
	if err != nil {
		t.Fatalf("Failed to insert pending step: %v", err)
	}
	return id
}

func newTestReaper(database *sql.DB) *Reaper {
	return &Reaper{
		DB:       database,
		Interval: time.Second,
		Logger:   slog.Default(),
	}
}

func TestReaper_ReapSteps_ExpiredLease(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	execID := createTestExecution(t, database)

	// Insert a step with an expired lease (1 second in the past).
	expiredAt := time.Now().Add(-1 * time.Second)
	stepID := insertRunningStep(t, database, execID, "expired-step", expiredAt)

	reaper := newTestReaper(database)
	count, err := reaper.ReapSteps(ctx)
	if err != nil {
		t.Fatalf("ReapSteps() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("ReapSteps() count = %d, want 1", count)
	}

	// Verify the step is now failed.
	var status, stepError string
	var claimedBy sql.NullString
	var leaseExpiresAt sql.NullTime
	err = database.QueryRow(`
		SELECT status, error, claimed_by, lease_expires_at
		FROM step_executions WHERE id = $1
	`, stepID).Scan(&status, &stepError, &claimedBy, &leaseExpiresAt)
	if err != nil {
		t.Fatalf("QueryRow error = %v", err)
	}
	if status != "failed" {
		t.Errorf("status = %q, want %q", status, "failed")
	}
	if stepError != "lease expired" {
		t.Errorf("error = %q, want %q", stepError, "lease expired")
	}
	if claimedBy.Valid {
		t.Errorf("claimed_by = %q, want NULL", claimedBy.String)
	}
	if leaseExpiresAt.Valid {
		t.Errorf("lease_expires_at should be NULL")
	}
}

func TestReaper_ReapSteps_IgnoresActiveLease(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	execID := createTestExecution(t, database)

	// Insert a step with a future lease (5 minutes from now).
	futureAt := time.Now().Add(5 * time.Minute)
	stepID := insertRunningStep(t, database, execID, "active-step", futureAt)

	reaper := newTestReaper(database)
	count, err := reaper.ReapSteps(ctx)
	if err != nil {
		t.Fatalf("ReapSteps() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("ReapSteps() count = %d, want 0", count)
	}

	// Verify the step is still running.
	var status string
	err = database.QueryRow(`
		SELECT status FROM step_executions WHERE id = $1
	`, stepID).Scan(&status)
	if err != nil {
		t.Fatalf("QueryRow error = %v", err)
	}
	if status != "running" {
		t.Errorf("status = %q, want %q", status, "running")
	}
}

func TestReaper_ReapExecutionClaims_Expired(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	execID := createTestExecution(t, database)

	// Insert an expired execution claim.
	expiredAt := time.Now().Add(-1 * time.Second)
	_, err := database.Exec(`
		INSERT INTO execution_claims (execution_id, claimed_by, lease_expires_at)
		VALUES ($1, 'node-1', $2)
	`, execID, expiredAt)
	if err != nil {
		t.Fatalf("Failed to insert execution claim: %v", err)
	}

	reaper := newTestReaper(database)
	count, err := reaper.ReapExecutionClaims(ctx)
	if err != nil {
		t.Fatalf("ReapExecutionClaims() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("ReapExecutionClaims() count = %d, want 1", count)
	}

	// Verify the claim is deleted.
	var exists bool
	err = database.QueryRow(`
		SELECT EXISTS (SELECT 1 FROM execution_claims WHERE execution_id = $1)
	`, execID).Scan(&exists)
	if err != nil {
		t.Fatalf("QueryRow error = %v", err)
	}
	if exists {
		t.Error("execution claim still exists after reaping")
	}
}

func TestReaper_ReapExecutionClaims_IgnoresActive(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	execID := createTestExecution(t, database)

	// Insert a claim with a future lease.
	futureAt := time.Now().Add(5 * time.Minute)
	_, err := database.Exec(`
		INSERT INTO execution_claims (execution_id, claimed_by, lease_expires_at)
		VALUES ($1, 'node-1', $2)
	`, execID, futureAt)
	if err != nil {
		t.Fatalf("Failed to insert execution claim: %v", err)
	}

	reaper := newTestReaper(database)
	count, err := reaper.ReapExecutionClaims(ctx)
	if err != nil {
		t.Fatalf("ReapExecutionClaims() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("ReapExecutionClaims() count = %d, want 0", count)
	}

	// Verify the claim still exists.
	var exists bool
	err = database.QueryRow(`
		SELECT EXISTS (SELECT 1 FROM execution_claims WHERE execution_id = $1)
	`, execID).Scan(&exists)
	if err != nil {
		t.Fatalf("QueryRow error = %v", err)
	}
	if !exists {
		t.Error("active execution claim was incorrectly reaped")
	}
}
