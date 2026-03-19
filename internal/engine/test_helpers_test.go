package engine

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/dvflw/mantle/internal/db"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// setupTestDB spins up a Postgres container, runs migrations, and returns
// a connected *sql.DB. The container is automatically cleaned up when the
// test finishes.
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

// createTestExecution inserts a workflow_execution row and returns its UUID.
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

// insertPendingStep inserts a pending step_execution and returns its UUID.
func insertPendingStep(t *testing.T, database *sql.DB, executionID, stepName string) string {
	t.Helper()
	var id string
	err := database.QueryRow(`
		INSERT INTO step_executions (execution_id, step_name, attempt, status)
		VALUES ($1, $2, 1, 'pending')
		RETURNING id
	`, executionID, stepName).Scan(&id)
	if err != nil {
		t.Fatalf("Failed to insert pending step: %v", err)
	}
	return id
}
