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
