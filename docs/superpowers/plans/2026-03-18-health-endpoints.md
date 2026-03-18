# Health Endpoints Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `/healthz` and `/readyz` HTTP handlers and a `database/sql` connection layer using pgx.

**Architecture:** `internal/db/` provides `Open()` and context helpers. `internal/api/health/` provides two HTTP handlers — `HealthzHandler` (always 200) and `ReadyzHandler` (pings Postgres). No Cobra integration — handlers are standalone, wired into a server by future `mantle serve`.

**Tech Stack:** Go, `net/http`, `database/sql`, `pgx/v5/stdlib`, `httptest`

**Spec:** `docs/superpowers/specs/2026-03-18-health-endpoints-design.md`

**Linear issue:** [DVFLW-274](https://linear.app/dvflw/issue/DVFLW-274/health-endpoints-healthz-and-readyz)

---

### Task 1: DB connection package

**Files:**
- Create: `internal/db/db.go`
- Test: `internal/db/db_test.go`

- [ ] **Step 1: Install pgx dependency**

Run:
```bash
go get github.com/jackc/pgx/v5
go mod tidy
```

- [ ] **Step 2: Write the failing test**

Create `internal/db/db_test.go`:

```go
package db

import (
	"context"
	"testing"
)

func TestOpen_InvalidURL(t *testing.T) {
	_, err := Open("not-a-valid-url")
	if err == nil {
		t.Fatal("Open() expected error for invalid URL, got nil")
	}
}

func TestContextHelpers(t *testing.T) {
	ctx := context.Background()

	// FromContext returns nil when no DB on context
	if got := FromContext(ctx); got != nil {
		t.Errorf("FromContext(empty) = %v, want nil", got)
	}

	// WithContext + FromContext round-trip
	// We can't create a real *sql.DB without Postgres,
	// but we can test the context plumbing with nil
	ctx = WithContext(ctx, nil)
	got := FromContext(ctx)
	if got != nil {
		t.Errorf("FromContext(with nil) = %v, want nil", got)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run:
```bash
go test ./internal/db/ -v
```

Expected: FAIL — package does not exist yet.

- [ ] **Step 4: Write implementation**

Create `internal/db/db.go`:

```go
package db

import (
	"context"
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type contextKey struct{}

// Open connects to Postgres using the given URL and verifies the connection.
func Open(databaseURL string) (*sql.DB, error) {
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}

	if err := database.Ping(); err != nil {
		database.Close()
		return nil, err
	}

	return database, nil
}

// WithContext returns a new context with the database attached.
func WithContext(ctx context.Context, database *sql.DB) context.Context {
	return context.WithValue(ctx, contextKey{}, database)
}

// FromContext retrieves the *sql.DB from context. Returns nil if not set.
func FromContext(ctx context.Context) *sql.DB {
	database, _ := ctx.Value(contextKey{}).(*sql.DB)
	return database
}
```

- [ ] **Step 5: Run tests**

Run:
```bash
go test ./internal/db/ -v
```

Expected: PASS — `TestOpen_InvalidURL` passes (pgx rejects bad URL), `TestContextHelpers` passes.

- [ ] **Step 6: Run all tests**

Run:
```bash
go test ./... -v
```

Expected: All tests pass across all packages.

- [ ] **Step 7: Commit**

```bash
git add internal/db/ go.mod go.sum
git commit -m "feat: add db package with Open() and context helpers using pgx"
```

---

### Task 2: Healthz handler

**Files:**
- Create: `internal/api/health/health.go`
- Test: `internal/api/health/health_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/api/health/health_test.go`:

```go
package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthzHandler(t *testing.T) {
	handler := HealthzHandler()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %q, want ok", body["status"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/api/health/ -v
```

Expected: FAIL — package does not exist yet.

- [ ] **Step 3: Write implementation**

Create `internal/api/health/health.go`:

```go
package health

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

type response struct {
	Status string `json:"status"`
}

// HealthzHandler returns an HTTP handler for liveness probes.
// Always returns 200 OK.
func HealthzHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response{Status: "ok"})
	}
}

// ReadyzHandler returns an HTTP handler for readiness probes.
// Returns 200 if Postgres is reachable, 503 otherwise.
// If database is nil, returns 503 immediately.
func ReadyzHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if database == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(response{Status: "unavailable"})
			return
		}

		if err := database.PingContext(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(response{Status: "unavailable"})
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response{Status: "ok"})
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/api/health/ -v -run TestHealthzHandler
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/health/
git commit -m "feat: add healthz handler — always returns 200 OK"
```

---

### Task 3: Readyz handler tests

**Files:**
- Modify: `internal/api/health/health_test.go`

- [ ] **Step 1: Add readyz tests**

Add to `internal/api/health/health_test.go`:

```go
func TestReadyzHandler_NilDB(t *testing.T) {
	handler := ReadyzHandler(nil)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}
	if body["status"] != "unavailable" {
		t.Errorf("status = %q, want unavailable", body["status"])
	}
}

func TestReadyzHandler_WithDB(t *testing.T) {
	// Use sqltest or a real DB if available
	// For unit tests, we can use a SQLite in-memory database
	// to verify the handler works with a real *sql.DB
	// However, since we only have pgx driver registered,
	// we test the nil case above and defer real DB testing to DVFLW-278.
	// This test verifies the handler returns JSON with correct content type
	// when DB is nil (already covered above).
	t.Skip("Real DB readyz test deferred to DVFLW-278 (testcontainers)")
}
```

- [ ] **Step 2: Run all health tests**

Run:
```bash
go test ./internal/api/health/ -v
```

Expected: PASS — `TestHealthzHandler` passes, `TestReadyzHandler_NilDB` passes, `TestReadyzHandler_WithDB` is skipped.

- [ ] **Step 3: Commit**

```bash
git add internal/api/health/health_test.go
git commit -m "test: add readyz handler tests — nil DB returns 503"
```

---

### Task 4: Final verification

- [ ] **Step 1: Run all tests**

Run:
```bash
go test ./... -v
```

Expected: All tests pass across all packages (db, health, cli, config, version).

- [ ] **Step 2: Run go vet**

Run:
```bash
go vet ./...
```

Expected: No warnings.

- [ ] **Step 3: Build and verify**

Run:
```bash
make build && ./mantle version && make clean
```

Expected: Version output works, no regressions.
