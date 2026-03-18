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

- Uses `lib/pq` driver (`github.com/lib/pq`)
- Calls `db.Ping()` to verify connection on open
- Returns `*sql.DB` with default pool settings (no tuning for now)

Context helpers (same pattern as config):

```go
type contextKey struct{}

func WithContext(ctx context.Context, database *sql.DB) context.Context
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

Calls `database.PingContext(r.Context())`:
- Success: HTTP 200 with `{"status":"ok"}`
- Failure: HTTP 503 with `{"status":"unavailable"}`

Content-Type: `application/json`.

## Cobra Integration

In `internal/cli/root.go`'s `PersistentPreRunE`, after loading config:

1. Call `db.Open(cfg.Database.URL)` to establish connection
2. Store on context via `db.WithContext()`

The version command already has a no-op `PersistentPreRunE` bypass, so it won't attempt a DB connection.

## Testing

- `health_test.go` — uses `httptest.NewRecorder`:
  - `/healthz` always returns 200 `{"status":"ok"}`
  - `/readyz` with nil DB returns 503
  - `/readyz` with working DB returns 200 (tested via a real or in-memory approach)
- `db_test.go` — unit test for `Open()` with invalid URL returns error. Integration test against real Postgres skipped unless `MANTLE_TEST_DATABASE_URL` is set (real Postgres testing comes in DVFLW-278).

## Dependencies

- `github.com/lib/pq` — Postgres driver for `database/sql` (must be added to go.mod)

## What's NOT Included

- CLI command to run health server (comes with `mantle serve` in Phase 5)
- Connection pooling configuration
- Integration tests against real Postgres (DVFLW-278)
- Authentication on health endpoints (explicitly excluded per acceptance criteria)
