# Design: Health Endpoints — /healthz and /readyz

> Linear issue: [DVFLW-274](https://linear.app/dvflw/issue/DVFLW-274/health-endpoints-healthz-and-readyz)
> Date: 2026-03-18

## Goal

Add `/healthz` and `/readyz` HTTP handlers for Kubernetes liveness and readiness probes. Also establish the `database/sql` connection layer used by readiness checks and all future database operations.

## Acceptance Criteria

- `/healthz` returns 200 when process is alive
- `/readyz` returns 200 when Postgres connection is healthy
- No authentication required on health endpoints

## Package Structure

```
internal/
  db/
    db.go              # Open() returns *sql.DB, context helpers
    db_test.go         # Unit test (skip if no Postgres)
  api/
    health/
      health.go        # /healthz and /readyz handlers
      health_test.go   # Tests using httptest
```

## internal/db/db.go

```go
// Open connects to Postgres using the given URL and verifies the connection.
func Open(databaseURL string) (*sql.DB, error)
```

- Uses `pgx` driver (`github.com/jackc/pgx/v5/stdlib`) — `lib/pq` is in maintenance mode; `pgx` is the actively maintained Postgres driver for Go
- Calls `db.Ping()` to verify connection on open
- Returns `*sql.DB` with default pool settings (no tuning for now)

Context helpers (same pattern as config):

```go
type contextKey struct{}

func WithContext(ctx context.Context, database *sql.DB) context.Context

// FromContext retrieves the *sql.DB from context. Returns nil if not set.
func FromContext(ctx context.Context) *sql.DB
```

## internal/api/health/health.go

Two handler constructors:

### HealthzHandler

```go
func HealthzHandler() http.HandlerFunc
```

Always returns HTTP 200 with `{"status":"ok"}`. Content-Type: `application/json`.

### ReadyzHandler

```go
func ReadyzHandler(database *sql.DB) http.HandlerFunc
```

If `database` is nil, returns 503 immediately without calling Ping. Otherwise calls `database.PingContext(r.Context())`:
- Success: HTTP 200 with `{"status":"ok"}`
- Failure: HTTP 503 with `{"status":"unavailable"}`

Content-Type: `application/json`.

## DB Connection Strategy

**DB connection is NOT in `PersistentPreRunE`.** Many commands (`validate`, `plan`, `version`) are offline and should not require Postgres. Instead:

- `internal/db/` provides `Open()` as a standalone function
- Commands that need a database call `db.Open()` in their own `RunE` and store on context if needed
- Future `mantle serve` will open the connection at startup and pass `*sql.DB` to health handlers and the engine

For this issue, the health handlers receive `*sql.DB` as an explicit parameter. No Cobra integration changes needed — the handlers are tested standalone with `httptest` and wired into a server when `mantle serve` is built.

### DB Lifecycle

`*sql.DB` is closed by whichever code opened it. For CLI commands, close with `defer db.Close()` after `Open()`. For `mantle serve`, close on graceful shutdown. This is not a concern for this issue since we're only building handlers, not a running server.

## Testing

- `health_test.go` — uses `httptest.NewRecorder`:
  - `/healthz` always returns 200 `{"status":"ok"}`
  - `/readyz` with nil DB returns 503
  - `/readyz` with working DB returns 200 (tested via a real or in-memory approach)
- `db_test.go` — unit test for `Open()` with invalid URL returns error. Integration test against real Postgres skipped unless `MANTLE_TEST_DATABASE_URL` is set (real Postgres testing comes in DVFLW-278).

## Dependencies

- `github.com/jackc/pgx/v5` — Postgres driver for `database/sql` via `pgx/v5/stdlib` (must be added to go.mod)

## What's NOT Included

- CLI command to run health server (comes with `mantle serve` in Phase 5)
- Connection pooling configuration
- Integration tests against real Postgres (DVFLW-278)
- Authentication on health endpoints (explicitly excluded per acceptance criteria)
