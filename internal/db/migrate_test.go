package db

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupTestDB(t *testing.T) string {
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
	return connStr
}

func TestMigrate(t *testing.T) {
	connStr := setupTestDB(t)
	database, err := Open(connStr)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer database.Close()

	ctx := context.Background()
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	tables := []string{"workflow_definitions", "workflow_executions", "step_executions"}
	for _, table := range tables {
		var exists bool
		err := database.QueryRow(
			"SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = $1)", table,
		).Scan(&exists)
		if err != nil {
			t.Fatalf("QueryRow error for %s: %v", table, err)
		}
		if !exists {
			t.Errorf("table %s does not exist after migration", table)
		}
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	connStr := setupTestDB(t)
	database, err := Open(connStr)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer database.Close()

	ctx := context.Background()
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate() first run error = %v", err)
	}
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate() second run error = %v", err)
	}
}

func TestMigrateDown(t *testing.T) {
	connStr := setupTestDB(t)
	database, err := Open(connStr)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer database.Close()

	ctx := context.Background()
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	// Roll back all migrations (one at a time).
	if err := MigrateDown(ctx, database); err != nil {
		t.Fatalf("MigrateDown() 1 error = %v", err)
	}
	if err := MigrateDown(ctx, database); err != nil {
		t.Fatalf("MigrateDown() 2 error = %v", err)
	}

	tables := []string{"workflow_definitions", "workflow_executions", "step_executions", "credentials"}
	for _, table := range tables {
		var exists bool
		err := database.QueryRow(
			"SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = $1)", table,
		).Scan(&exists)
		if err != nil {
			t.Fatalf("QueryRow error for %s: %v", table, err)
		}
		if exists {
			t.Errorf("table %s still exists after migrate down", table)
		}
	}
}

func TestMigrateStatus(t *testing.T) {
	connStr := setupTestDB(t)
	database, err := Open(connStr)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer database.Close()

	ctx := context.Background()
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	status, err := MigrateStatus(ctx, database)
	if err != nil {
		t.Fatalf("MigrateStatus() error = %v", err)
	}
	if status == "" {
		t.Error("MigrateStatus() returned empty string")
	}
	if !strings.Contains(status, "001_initial_schema") {
		t.Errorf("MigrateStatus() output missing migration name, got: %s", status)
	}
}
