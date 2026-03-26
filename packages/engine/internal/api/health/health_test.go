package health

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeDriver and fakeConn implement a minimal sql/driver that supports Ping.
type fakeDriver struct{}
type fakeConn struct{}

func (d fakeDriver) Open(name string) (driver.Conn, error) { return fakeConn{}, nil }
func (c fakeConn) Prepare(query string) (driver.Stmt, error) {
	return nil, driver.ErrSkip
}
func (c fakeConn) Close() error                                         { return nil }
func (c fakeConn) Begin() (driver.Tx, error)                            { return nil, driver.ErrSkip }
func (c fakeConn) Ping(ctx context.Context) error                       { return nil }
func (c fakeConn) IsValid() bool                                        { return true }
func (c fakeConn) ResetSession(ctx context.Context) error               { return nil }

func init() {
	sql.Register("fakedb", fakeDriver{})
}

func openFakeDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("fakedb", "")
	if err != nil {
		t.Fatalf("failed to open fake db: %v", err)
	}
	return db
}

type mockChecker struct {
	alive bool
	name  string
}

func (m *mockChecker) IsAlive() bool { return m.alive }
func (m *mockChecker) Name() string  { return m.name }

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

func TestReadyzHandler_AllHealthy(t *testing.T) {
	db := openFakeDB(t)
	defer db.Close()

	handler := ReadyzHandler(db,
		&mockChecker{alive: true, name: "worker"},
		&mockChecker{alive: true, name: "reaper"},
	)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body response
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status = %q, want ok", body.Status)
	}
	if body.Details["worker"] != "ok" {
		t.Errorf("worker detail = %q, want ok", body.Details["worker"])
	}
	if body.Details["reaper"] != "ok" {
		t.Errorf("reaper detail = %q, want ok", body.Details["reaper"])
	}
}

func TestReadyzHandler_DegradedWorker(t *testing.T) {
	db := openFakeDB(t)
	defer db.Close()

	handler := ReadyzHandler(db,
		&mockChecker{alive: true, name: "worker"},
		&mockChecker{alive: false, name: "reaper"},
	)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	var body response
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if body.Status != "degraded" {
		t.Errorf("status = %q, want degraded", body.Status)
	}
	if body.Details["worker"] != "ok" {
		t.Errorf("worker detail = %q, want ok", body.Details["worker"])
	}
	if body.Details["reaper"] != "degraded" {
		t.Errorf("reaper detail = %q, want degraded", body.Details["reaper"])
	}
}

func TestReadyzHandler_NoCheckers(t *testing.T) {
	db := openFakeDB(t)
	defer db.Close()

	handler := ReadyzHandler(db)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body response
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status = %q, want ok", body.Status)
	}
}
