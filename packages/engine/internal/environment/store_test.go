package environment

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/dvflw/mantle/internal/dbdefaults"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	pgContainer, err := postgres.Run(ctx,
		dbdefaults.PostgresImage,
		postgres.WithDatabase(dbdefaults.TestDatabase),
		postgres.WithUsername(dbdefaults.User),
		postgres.WithPassword(dbdefaults.Password),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("Could not start Postgres container (CI): %v", err)
		}
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
		t.Fatalf("Failed to open database: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := db.Migrate(ctx, database); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	return database
}

func TestStore_CreateAndGet(t *testing.T) {
	database := setupTestDB(t)
	store := &Store{DB: database}
	ctx := context.Background()

	inputs := map[string]any{"url": "https://example.com", "retries": float64(3)}
	env := map[string]string{"LOG_LEVEL": "debug", "REGION": "us-east-1"}

	created, err := store.Create(ctx, "staging", inputs, env)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if created.ID == "" {
		t.Error("Create() returned empty ID")
	}
	if created.Name != "staging" {
		t.Errorf("Create() name = %q, want %q", created.Name, "staging")
	}
	if created.CreatedAt.IsZero() {
		t.Error("Create() returned zero CreatedAt")
	}
	if created.UpdatedAt.IsZero() {
		t.Error("Create() returned zero UpdatedAt")
	}

	got, err := store.Get(ctx, "staging")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("Get() ID = %q, want %q", got.ID, created.ID)
	}
	if got.Name != "staging" {
		t.Errorf("Get() name = %q, want %q", got.Name, "staging")
	}
	if got.Inputs["url"] != "https://example.com" {
		t.Errorf("Get() inputs[url] = %v, want %q", got.Inputs["url"], "https://example.com")
	}
	if got.Env["LOG_LEVEL"] != "debug" {
		t.Errorf("Get() env[LOG_LEVEL] = %q, want %q", got.Env["LOG_LEVEL"], "debug")
	}
	if got.Env["REGION"] != "us-east-1" {
		t.Errorf("Get() env[REGION] = %q, want %q", got.Env["REGION"], "us-east-1")
	}
}

func TestStore_GetNotFound(t *testing.T) {
	database := setupTestDB(t)
	store := &Store{DB: database}
	ctx := context.Background()

	_, err := store.Get(ctx, "nonexistent")
	if err == nil {
		t.Fatal("Get() expected error for nonexistent environment, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Get() error = %q, want to contain %q", err.Error(), "not found")
	}
}

func TestStore_List(t *testing.T) {
	database := setupTestDB(t)
	store := &Store{DB: database}
	ctx := context.Background()

	_, err := store.Create(ctx, "production", map[string]any{"tier": "prod"}, map[string]string{"ENV": "prod"})
	if err != nil {
		t.Fatalf("Create() production error: %v", err)
	}
	_, err = store.Create(ctx, "staging", map[string]any{"tier": "staging"}, map[string]string{"ENV": "staging"})
	if err != nil {
		t.Fatalf("Create() staging error: %v", err)
	}

	envs, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(envs) != 2 {
		t.Fatalf("List() returned %d environments, want 2", len(envs))
	}
	// List returns ordered by name: production < staging
	if envs[0].Name != "production" {
		t.Errorf("List()[0].Name = %q, want %q", envs[0].Name, "production")
	}
	if envs[1].Name != "staging" {
		t.Errorf("List()[1].Name = %q, want %q", envs[1].Name, "staging")
	}
}

func TestStore_Delete(t *testing.T) {
	database := setupTestDB(t)
	store := &Store{DB: database}
	ctx := context.Background()

	_, err := store.Create(ctx, "dev", nil, nil)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	if err := store.Delete(ctx, "dev"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	_, err = store.Get(ctx, "dev")
	if err == nil {
		t.Fatal("Get() after Delete() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Get() error = %q, want to contain %q", err.Error(), "not found")
	}
}

func TestStore_DuplicateName(t *testing.T) {
	database := setupTestDB(t)
	store := &Store{DB: database}
	ctx := context.Background()

	_, err := store.Create(ctx, "duplicate", nil, nil)
	if err != nil {
		t.Fatalf("Create() first error: %v", err)
	}

	_, err = store.Create(ctx, "duplicate", nil, nil)
	if err == nil {
		t.Fatal("Create() second call expected error for duplicate name, got nil")
	}
}

func TestStore_InvalidName(t *testing.T) {
	store := &Store{DB: setupTestDB(t)}
	ctx := context.Background()

	// Space, uppercase, symbols — still rejected.
	for _, bad := range []string{"", "Invalid Name!", "has space", "UPPER", "-leading", "_trailing_"} {
		_, err := store.Create(ctx, bad, nil, nil)
		if err == nil {
			t.Errorf("expected error for invalid name %q", bad)
		}
	}

	// Length cap: exactly 63 chars is allowed, 64 is rejected.
	longOK := strings.Repeat("a", 63)
	if _, err := store.Create(ctx, longOK, nil, nil); err != nil {
		t.Errorf("expected 63-char name to be accepted, got error: %v", err)
	}
	tooLong := strings.Repeat("a", 64)
	if _, err := store.Create(ctx, tooLong, nil, nil); err == nil {
		t.Errorf("expected 64-char name to be rejected")
	}
}

func TestStore_ValidNameRelaxed(t *testing.T) {
	store := &Store{DB: setupTestDB(t)}
	ctx := context.Background()

	// Relaxed regex allows digits first, underscores, hyphens.
	for _, good := range []string{"prod_1", "env-v2", "123env", "a"} {
		if _, err := store.Create(ctx, good, nil, nil); err != nil {
			t.Errorf("expected %q to be valid, got error: %v", good, err)
		}
	}
}

func TestStore_Update(t *testing.T) {
	database := setupTestDB(t)
	store := &Store{DB: database}
	ctx := context.Background()

	created, err := store.Create(ctx, "staging", map[string]any{"url": "old"}, map[string]string{"K": "v1"})
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	updated, err := store.Update(ctx, "staging", map[string]any{"url": "new"}, map[string]string{"K": "v2", "NEW": "x"})
	if err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	if updated.ID != created.ID {
		t.Errorf("Update() changed ID: %q -> %q", created.ID, updated.ID)
	}
	if !updated.CreatedAt.Equal(created.CreatedAt) {
		t.Errorf("Update() changed CreatedAt: %v -> %v", created.CreatedAt, updated.CreatedAt)
	}
	// Assert non-decreasing rather than strictly after: Postgres NOW() can
	// return the same value for two quickly-successive transactions on some
	// systems. Strict inequality made this test wall-clock dependent.
	if updated.UpdatedAt.Before(created.UpdatedAt) {
		t.Errorf("Update() moved UpdatedAt backwards: created=%v updated=%v", created.UpdatedAt, updated.UpdatedAt)
	}

	got, err := store.Get(ctx, "staging")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got.Inputs["url"] != "new" {
		t.Errorf("Get() inputs[url] = %v, want %q", got.Inputs["url"], "new")
	}
	if got.Env["K"] != "v2" || got.Env["NEW"] != "x" {
		t.Errorf("Get() env = %v, want K=v2 NEW=x", got.Env)
	}
}

func TestStore_UpdateNotFound(t *testing.T) {
	store := &Store{DB: setupTestDB(t)}
	ctx := context.Background()

	_, err := store.Update(ctx, "missing", nil, nil)
	if err == nil {
		t.Fatal("Update() expected error for missing environment")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Update() error = %q, want to contain 'not found'", err.Error())
	}
}

func TestStore_AuditEvents(t *testing.T) {
	database := setupTestDB(t)
	store := &Store{DB: database}
	ctx := context.Background()

	created, err := store.Create(ctx, "audited", map[string]any{"k": "v"}, nil)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if _, err := store.Update(ctx, "audited", map[string]any{"k": "v2"}, nil); err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	if err := store.EmitReveal(ctx, created); err != nil {
		t.Fatalf("EmitReveal() error: %v", err)
	}
	if err := store.Delete(ctx, "audited"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	want := map[string]int{
		"environment.created":  1,
		"environment.updated":  1,
		"environment.revealed": 1,
		"environment.deleted":  1,
	}
	for action, n := range want {
		var count int
		row := database.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM audit_events WHERE action = $1 AND resource_type = 'environment' AND resource_id = $2`,
			action, created.ID,
		)
		if err := row.Scan(&count); err != nil {
			t.Fatalf("query audit count for %s: %v", action, err)
		}
		if count != n {
			t.Errorf("audit events for %s = %d, want %d", action, count, n)
		}
	}
}
