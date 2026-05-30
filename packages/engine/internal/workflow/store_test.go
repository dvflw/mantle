package workflow

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"os"
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

var testWorkflowYAML = []byte(`name: test-workflow
description: A test workflow

inputs:
  url:
    type: string

steps:
  - name: fetch
    action: http/request
    params:
      method: GET
      url: "{{ inputs.url }}"
`)

var testWorkflowYAMLModified = []byte(`name: test-workflow
description: A modified test workflow

inputs:
  url:
    type: string

steps:
  - name: fetch
    action: http/request
    params:
      method: POST
      url: "{{ inputs.url }}"
`)

func TestSave_NewWorkflow(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	result, err := ParseBytes(testWorkflowYAML)
	if err != nil {
		t.Fatalf("ParseBytes() error = %v", err)
	}

	version, err := Save(ctx, database, result, testWorkflowYAML)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if version != 1 {
		t.Errorf("Save() version = %d, want 1", version)
	}
}

func TestSave_NoChanges(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	result, err := ParseBytes(testWorkflowYAML)
	if err != nil {
		t.Fatalf("ParseBytes() error = %v", err)
	}

	_, err = Save(ctx, database, result, testWorkflowYAML)
	if err != nil {
		t.Fatalf("Save() first call error = %v", err)
	}

	version, err := Save(ctx, database, result, testWorkflowYAML)
	if err != nil {
		t.Fatalf("Save() second call error = %v", err)
	}
	if version != 0 {
		t.Errorf("Save() version = %d, want 0 (no changes)", version)
	}
}

func TestSave_NewVersion(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	result1, err := ParseBytes(testWorkflowYAML)
	if err != nil {
		t.Fatalf("ParseBytes() error = %v", err)
	}
	_, err = Save(ctx, database, result1, testWorkflowYAML)
	if err != nil {
		t.Fatalf("Save() first call error = %v", err)
	}

	result2, err := ParseBytes(testWorkflowYAMLModified)
	if err != nil {
		t.Fatalf("ParseBytes() error = %v", err)
	}
	version, err := Save(ctx, database, result2, testWorkflowYAMLModified)
	if err != nil {
		t.Fatalf("Save() second call error = %v", err)
	}
	if version != 2 {
		t.Errorf("Save() version = %d, want 2", version)
	}
}

func TestSave_CorrectHash(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	result, err := ParseBytes(testWorkflowYAML)
	if err != nil {
		t.Fatalf("ParseBytes() error = %v", err)
	}

	_, err = Save(ctx, database, result, testWorkflowYAML)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	expectedHash := sha256.Sum256(testWorkflowYAML)
	expectedHashStr := hex.EncodeToString(expectedHash[:])

	storedHash, err := GetLatestHash(ctx, database, "test-workflow")
	if err != nil {
		t.Fatalf("GetLatestHash() error = %v", err)
	}
	if storedHash != expectedHashStr {
		t.Errorf("stored hash = %q, want %q", storedHash, expectedHashStr)
	}
}

func TestGetLatestVersion_NoVersions(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	version, err := GetLatestVersion(ctx, database, "nonexistent-workflow")
	if err != nil {
		t.Fatalf("GetLatestVersion() error = %v", err)
	}
	if version != 0 {
		t.Errorf("GetLatestVersion() = %d, want 0", version)
	}
}

func TestDisable_MarksLatestVersion(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()
	raw := []byte("name: wf\nsteps:\n  - name: s\n    action: http/request\n")
	result, err := ParseBytes(raw)
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	if _, err := Save(ctx, database, result, raw); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := Disable(ctx, database, "wf"); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	var disabled sql.NullTime
	if err := database.QueryRowContext(ctx,
		`SELECT disabled_at FROM workflow_definitions WHERE name = 'wf' ORDER BY version DESC LIMIT 1`,
	).Scan(&disabled); err != nil {
		t.Fatalf("query: %v", err)
	}
	if !disabled.Valid {
		t.Error("disabled_at should be non-null after Disable")
	}
}

func TestReenable_ClearsDisabledAt(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()
	raw := []byte("name: wf2\nsteps:\n  - name: s\n    action: http/request\n")
	result, _ := ParseBytes(raw)
	if _, err := Save(ctx, database, result, raw); err != nil {
		t.Fatalf("Save: %v", err)
	}
	_ = Disable(ctx, database, "wf2")
	if err := Reenable(ctx, database, "wf2"); err != nil {
		t.Fatalf("Reenable: %v", err)
	}
	var disabled sql.NullTime
	_ = database.QueryRowContext(ctx,
		`SELECT disabled_at FROM workflow_definitions WHERE name = 'wf2' ORDER BY version DESC LIMIT 1`,
	).Scan(&disabled)
	if disabled.Valid {
		t.Error("disabled_at should be null after Reenable")
	}
}
