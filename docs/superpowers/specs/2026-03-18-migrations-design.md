# Design: Postgres Connection and Migration System

> Linear issue: [DVFLW-219](https://linear.app/dvflw/issue/DVFLW-219/postgres-connection-and-migration-system)
> Date: 2026-03-18

## Goal

Add a migration system using goose, create the initial Phase 1 schema (workflow_definitions, workflow_executions, step_executions), and add `mantle init`, `mantle migrate`, `mantle migrate status`, and `mantle migrate down` CLI commands.

## Acceptance Criteria

- `mantle init` runs migrations and creates the schema
- Migrations are versioned and idempotent
- Connection pooling configured appropriately

## Package Structure

```
internal/
  db/
    db.go              # Already exists — Open(), context helpers
    migrate.go         # Goose wrapper with embedded migrations
    migrate_test.go    # Tests with testcontainers
  migrations/
    001_initial_schema.sql   # Creates all Phase 1 tables
  cli/
    init.go            # mantle init command
    migrate.go         # mantle migrate, migrate status, migrate down
```

## Migration Runner

Uses `github.com/pressly/goose/v3` with SQL files embedded via `//go:embed`.

### `internal/db/migrate.go`

```go
// Migrate runs all pending migrations.
func Migrate(database *sql.DB) error

// MigrateDown rolls back the last applied migration.
func MigrateDown(database *sql.DB) error

// MigrateStatus prints migration status to the given writer.
func MigrateStatus(database *sql.DB, w io.Writer) error
```

Implementation:
- Set goose dialect to `postgres` via `goose.SetDialect("postgres")`
- Set base FS to embedded migrations via `goose.SetBaseFS()`
- `Migrate` calls `goose.Up()`
- `MigrateDown` calls `goose.Down()` (single step rollback)
- `MigrateStatus` calls `goose.Status()` which prints to the writer

Goose automatically manages its `goose_db_version` tracking table.

## Initial Migration: `001_initial_schema.sql`

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
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE step_executions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id UUID NOT NULL REFERENCES workflow_executions(id),
    step_name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    output JSONB,
    error TEXT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS step_executions;
DROP TABLE IF EXISTS workflow_executions;
DROP TABLE IF EXISTS workflow_definitions;
```

## CLI Commands

All migration commands open their own DB connection in `RunE` and close with `defer`. Config is loaded by the root command's `PersistentPreRunE`.

### `mantle init`

Full onboarding command. Runs all pending migrations. Future-proofed to also handle any other first-time setup tasks.

```
$ mantle init
Running migrations...
OK    001_initial_schema.sql
Migrations complete.
```

### `mantle migrate`

Runs all pending migrations. Semantically "upgrade" — same as init's migration step, but for ongoing upgrades.

```
$ mantle migrate
OK    002_add_indexes.sql
Migrations complete.
```

### `mantle migrate status`

Shows applied and pending migrations.

```
$ mantle migrate status
    Applied At                  Migration
    =======================================
    2026-03-18 20:00:00 +0000   001_initial_schema.sql
    Pending                     002_add_indexes.sql
```

### `mantle migrate down`

Rolls back the last applied migration (single step).

```
$ mantle migrate down
Rolled back 001_initial_schema.sql
```

## DB Connection

All commands use the existing `db.Open()` from `internal/db/db.go`. The `database.url` config value (from mantle.yaml, env var, or CLI flag) provides the connection string.

No connection pooling changes for now — `database/sql` defaults are adequate. Pool tuning can be added to config when needed.

## Testing

- `migrate_test.go` — uses testcontainers to spin up real Postgres:
  - Run `Migrate()`, verify all three tables exist via `information_schema.tables`
  - Run `Migrate()` again, verify idempotent (no error, no duplicate tables)
  - Run `MigrateDown()`, verify tables are dropped
  - Skip if Docker is unavailable
- CLI tests — unit test that init/migrate commands call the right functions (using the existing test pattern of creating a root command and executing with args)

## Dependencies

- `github.com/pressly/goose/v3` — migration runner (must be added to go.mod)
- `github.com/testcontainers/testcontainers-go` — for integration tests (test dependency)
- `github.com/testcontainers/testcontainers-go/modules/postgres` — Postgres testcontainer module

## What's NOT Included

- Migration generator (users can use `goose create` CLI directly)
- Seed data
- Indexes beyond primary keys and the unique constraint (add when query patterns emerge)
- Connection pool tuning
