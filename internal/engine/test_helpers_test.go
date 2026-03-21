package engine

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/dvflw/mantle/internal/config"
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

	database, err := db.Open(config.DatabaseConfig{URL: connStr})
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
