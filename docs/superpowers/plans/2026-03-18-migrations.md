# Migrations System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add goose-based migration system with initial Phase 1 schema and CLI commands (`init`, `migrate`, `migrate status`, `migrate down`).

**Architecture:** `internal/db/migrate.go` wraps goose provider API with embedded SQL files. `internal/migrations/` holds SQL migration files. CLI commands in `internal/cli/` open DB, call migration functions, close DB.

**Tech Stack:** Go, goose v3 (provider API), pgx, testcontainers-go

**Spec:** `docs/superpowers/specs/2026-03-18-migrations-design.md`

**Linear issue:** [DVFLW-219](https://linear.app/dvflw/issue/DVFLW-219/postgres-connection-and-migration-system)

---

### Task 1: Initial migration SQL file

**Files:**
- Create: `internal/migrations/001_initial_schema.sql`

- [ ] **Step 1: Create migrations directory and SQL file**

Create `internal/migrations/001_initial_schema.sql`:

```sql
-- +goose Up
CREATE TABLE workflow_definitions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    version INTEGER NOT NULL,
    content JSONB NOT NULL,
    content_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(name, version)
);

CREATE TABLE workflow_executions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_name TEXT NOT NULL,
    workflow_version INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    inputs JSONB,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE step_executions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id UUID NOT NULL REFERENCES workflow_executions(id),
    step_name TEXT NOT NULL,
    attempt INTEGER NOT NULL DEFAULT 1,
    status TEXT NOT NULL DEFAULT 'pending',
    output JSONB,
    error TEXT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(execution_id, step_name, attempt)
);

-- +goose Down
DROP TABLE IF EXISTS step_executions;
DROP TABLE IF EXISTS workflow_executions;
DROP TABLE IF EXISTS workflow_definitions;
```

- [ ] **Step 2: Commit**

```bash
git add internal/migrations/
git commit -m "feat: add initial schema migration with Phase 1 tables"
```

---

### Task 2: Migration runner (goose wrapper)

**Files:**
- Create: `internal/db/migrate.go`
- Test: `internal/db/migrate_test.go`

- [ ] **Step 1: Install goose and testcontainers dependencies**

Run:
```bash
go get github.com/pressly/goose/v3
go get github.com/testcontainers/testcontainers-go
go get github.com/testcontainers/testcontainers-go/modules/postgres
go mod tidy
```

- [ ] **Step 2: Write the failing test**

Create `internal/db/migrate_test.go`:

```go
package db

import (
	"context"
	"fmt"
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

	// Run migrations
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	// Verify all three tables exist
	tables := []string{"workflow_definitions", "workflow_executions", "step_executions"}
	for _, table := range tables {
		var exists bool
		err := database.QueryRow(
			"SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = $1)",
			table,
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

	// Run twice — should not error
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

	// Migrate up first
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	// Migrate down
	if err := MigrateDown(ctx, database); err != nil {
		t.Fatalf("MigrateDown() error = %v", err)
	}

	// Verify tables are gone
	tables := []string{"workflow_definitions", "workflow_executions", "step_executions"}
	for _, table := range tables {
		var exists bool
		err := database.QueryRow(
			"SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = $1)",
			table,
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

	// Run migrations first
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	// Get status
	status, err := MigrateStatus(ctx, database)
	if err != nil {
		t.Fatalf("MigrateStatus() error = %v", err)
	}

	if status == "" {
		t.Error("MigrateStatus() returned empty string")
	}

	// Should mention our migration
	if !contains(status, "001_initial_schema") {
		t.Errorf("MigrateStatus() output missing migration name, got: %s", status)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
```

- [ ] **Step 3: Run test to verify it fails**

Run:
```bash
go test ./internal/db/ -v -run TestMigrate
```

Expected: FAIL — `Migrate`, `MigrateDown`, `MigrateStatus` functions don't exist.

- [ ] **Step 4: Write implementation**

Create `internal/db/migrate.go`:

```go
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"strings"

	"github.com/pressly/goose/v3"
)

//go:embed ../migrations/*.sql
var migrations embed.FS

func newProvider(database *sql.DB) (*goose.Provider, error) {
	return goose.NewProvider(goose.DialectPostgres, database, migrations)
}

// Migrate runs all pending migrations.
func Migrate(ctx context.Context, database *sql.DB) error {
	provider, err := newProvider(database)
	if err != nil {
		return fmt.Errorf("creating migration provider: %w", err)
	}

	_, err = provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	return nil
}

// MigrateDown rolls back the last applied migration.
func MigrateDown(ctx context.Context, database *sql.DB) error {
	provider, err := newProvider(database)
	if err != nil {
		return fmt.Errorf("creating migration provider: %w", err)
	}

	_, err = provider.Down(ctx)
	if err != nil {
		return fmt.Errorf("rolling back migration: %w", err)
	}

	return nil
}

// MigrateStatus returns migration status as formatted text.
func MigrateStatus(ctx context.Context, database *sql.DB) (string, error) {
	provider, err := newProvider(database)
	if err != nil {
		return "", fmt.Errorf("creating migration provider: %w", err)
	}

	results, err := provider.Status(ctx)
	if err != nil {
		return "", fmt.Errorf("getting migration status: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%-30s  %s\n", "Applied At", "Migration")
	fmt.Fprintf(&b, "%s\n", strings.Repeat("=", 60))
	for _, r := range results {
		if r.State == goose.StateApplied {
			fmt.Fprintf(&b, "%-30s  %s\n", r.AppliedAt.Format("2006-01-02 15:04:05 -0700"), r.Source.Path)
		} else {
			fmt.Fprintf(&b, "%-30s  %s\n", "Pending", r.Source.Path)
		}
	}

	return b.String(), nil
}
```

- [ ] **Step 5: Run tests**

Run:
```bash
go test ./internal/db/ -v -timeout 120s
```

Expected: PASS — all migration tests pass (requires Docker for testcontainers). If Docker is unavailable, tests are skipped.

- [ ] **Step 6: Run all tests**

Run:
```bash
go test ./... -v -timeout 120s
```

Expected: All tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/db/migrate.go internal/db/migrate_test.go go.mod go.sum
git commit -m "feat: add goose-based migration runner with testcontainers tests"
```

---

### Task 3: `mantle init` CLI command

**Files:**
- Create: `internal/cli/init.go`
- Modify: `internal/cli/root.go` (register init command)

- [ ] **Step 1: Create init command**

Create `internal/cli/init.go`:

```go
package cli

import (
	"fmt"

	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/spf13/cobra"
)

func newInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize Mantle — run database migrations",
		Long:  "Runs all pending database migrations to set up or upgrade the Mantle schema.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.FromContext(cmd.Context())
			if cfg == nil {
				return fmt.Errorf("config not loaded")
			}

			database, err := db.Open(cfg.Database.URL)
			if err != nil {
				return fmt.Errorf("failed to connect to database: %w", err)
			}
			defer database.Close()

			fmt.Fprintln(cmd.OutOrStdout(), "Running migrations...")
			if err := db.Migrate(cmd.Context(), database); err != nil {
				return fmt.Errorf("migration failed: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Migrations complete.")
			return nil
		},
	}
}
```

- [ ] **Step 2: Register init command on root**

Modify `internal/cli/root.go` — add after the existing `cmd.AddCommand(newVersionCommand())` line:

```go
	cmd.AddCommand(newInitCommand())
```

- [ ] **Step 3: Verify it compiles**

Run:
```bash
go build ./cmd/mantle
```

Expected: Builds successfully.

- [ ] **Step 4: Verify help output**

Run:
```bash
go run ./cmd/mantle --help
```

Expected: Shows `init` in available commands list.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/init.go internal/cli/root.go
git commit -m "feat: add mantle init command for running migrations"
```

---

### Task 4: `mantle migrate` CLI commands

**Files:**
- Create: `internal/cli/migrate.go`
- Modify: `internal/cli/root.go` (register migrate command)

- [ ] **Step 1: Create migrate commands**

Create `internal/cli/migrate.go`:

```go
package cli

import (
	"fmt"

	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/spf13/cobra"
)

func newMigrateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run pending database migrations",
		Long:  "Runs all pending database migrations to upgrade the Mantle schema.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.FromContext(cmd.Context())
			if cfg == nil {
				return fmt.Errorf("config not loaded")
			}

			database, err := db.Open(cfg.Database.URL)
			if err != nil {
				return fmt.Errorf("failed to connect to database: %w", err)
			}
			defer database.Close()

			if err := db.Migrate(cmd.Context(), database); err != nil {
				return fmt.Errorf("migration failed: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Migrations complete.")
			return nil
		},
	}

	cmd.AddCommand(newMigrateStatusCommand())
	cmd.AddCommand(newMigrateDownCommand())

	return cmd
}

func newMigrateStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show migration status",
		Long:  "Shows which migrations have been applied and which are pending.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.FromContext(cmd.Context())
			if cfg == nil {
				return fmt.Errorf("config not loaded")
			}

			database, err := db.Open(cfg.Database.URL)
			if err != nil {
				return fmt.Errorf("failed to connect to database: %w", err)
			}
			defer database.Close()

			status, err := db.MigrateStatus(cmd.Context(), database)
			if err != nil {
				return fmt.Errorf("failed to get migration status: %w", err)
			}

			fmt.Fprint(cmd.OutOrStdout(), status)
			return nil
		},
	}
}

func newMigrateDownCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "down",
		Short: "Roll back the last migration",
		Long:  "Rolls back the most recently applied database migration.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.FromContext(cmd.Context())
			if cfg == nil {
				return fmt.Errorf("config not loaded")
			}

			database, err := db.Open(cfg.Database.URL)
			if err != nil {
				return fmt.Errorf("failed to connect to database: %w", err)
			}
			defer database.Close()

			if err := db.MigrateDown(cmd.Context(), database); err != nil {
				return fmt.Errorf("rollback failed: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Rollback complete.")
			return nil
		},
	}
}
```

- [ ] **Step 2: Register migrate command on root**

Modify `internal/cli/root.go` — add after the init command registration:

```go
	cmd.AddCommand(newMigrateCommand())
```

- [ ] **Step 3: Verify help output**

Run:
```bash
go run ./cmd/mantle --help
```

Expected: Shows `init` and `migrate` in available commands.

Run:
```bash
go run ./cmd/mantle migrate --help
```

Expected: Shows `status` and `down` as subcommands of `migrate`.

- [ ] **Step 4: Run all tests**

Run:
```bash
go test ./... -v -timeout 120s
```

Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/migrate.go internal/cli/root.go
git commit -m "feat: add mantle migrate, migrate status, migrate down commands"
```

---

### Task 5: Update Makefile migrate target

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Update migrate target**

Replace the existing placeholder `migrate` target in the Makefile:

From:
```makefile
migrate:
	@echo "No migrations yet. Run 'mantle init' when available."
```

To:
```makefile
migrate:
	go run ./cmd/mantle init
```

- [ ] **Step 2: Commit**

```bash
git add Makefile
git commit -m "chore: update Makefile migrate target to run mantle init"
```

---

### Task 6: Final verification

- [ ] **Step 1: Run all tests**

Run:
```bash
go test ./... -v -timeout 120s
```

Expected: All tests pass.

- [ ] **Step 2: Run go vet**

Run:
```bash
go vet ./...
```

Expected: No warnings.

- [ ] **Step 3: Build and verify CLI**

Run:
```bash
make build
./mantle --help
./mantle init --help
./mantle migrate --help
./mantle migrate status --help
./mantle migrate down --help
./mantle version
make clean
```

Expected: All help outputs show correct descriptions. Version still works.
