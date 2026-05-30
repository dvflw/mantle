package sync

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/dvflw/mantle/internal/dbdefaults"
	"github.com/dvflw/mantle/internal/repo"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupEngineTest(t *testing.T) (*sql.DB, *repo.Store) {
	t.Helper()
	ctx := context.Background()
	pg, err := postgres.Run(ctx,
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
			t.Fatalf("Postgres (CI): %v", err)
		}
		t.Skipf("Postgres: %v", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })
	connStr, _ := pg.ConnectionString(ctx, "sslmode=disable")
	database, err := db.Open(config.DatabaseConfig{URL: connStr})
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(ctx, database); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}
	return database, &repo.Store{DB: database, Actor: "test"}
}

// validWorkflowYAML returns a minimal valid workflow YAML for name.
// Must pass workflow.ParseBytes + workflow.Validate without error.
func validWorkflowYAML(name string) string {
	return "name: " + name + "\nsteps:\n  - name: hello\n    action: http/request\n"
}

func TestSyncRepo_AppliesNewFiles(t *testing.T) {
	database, store := setupEngineTest(t)
	ctx := context.Background()
	r, err := store.Create(ctx, repo.CreateParams{
		Name: "acme", URL: "https://example.com/a.git", Branch: "main",
		Path: "/", PollInterval: "60s", Credential: "pat", AutoApply: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	base := t.TempDir()
	repoDir := filepath.Join(base, r.ID)
	_ = os.MkdirAll(repoDir, 0o755)
	if err := os.WriteFile(filepath.Join(repoDir, "wf1.yaml"), []byte(validWorkflowYAML("wf1")), 0o644); err != nil {
		t.Fatalf("write wf1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "wf2.yaml"), []byte(validWorkflowYAML("wf2")), 0o644); err != nil {
		t.Fatalf("write wf2: %v", err)
	}

	driver := &NoopDriver{BasePath: base, SHA: "sha-1"}
	report, err := SyncRepo(ctx, database, store, r, driver)
	if err != nil {
		t.Fatalf("SyncRepo: %v", err)
	}
	if report.Applied != 2 {
		t.Errorf("Applied: got %d, want 2", report.Applied)
	}
	if report.Unchanged != 0 {
		t.Errorf("Unchanged: got %d, want 0", report.Unchanged)
	}
	if len(report.Failures) != 0 {
		t.Errorf("Failures: got %+v, want none", report.Failures)
	}
	if report.SHA != "sha-1" {
		t.Errorf("SHA: got %q, want sha-1", report.SHA)
	}

	got, _ := store.Get(ctx, "acme")
	if got.LastSyncSHA != "sha-1" {
		t.Errorf("LastSyncSHA not recorded: %q", got.LastSyncSHA)
	}
	if got.LastSyncError != "" {
		t.Errorf("LastSyncError should be empty, got %q", got.LastSyncError)
	}
}

func TestSyncRepo_UnchangedYieldsNoApply(t *testing.T) {
	database, store := setupEngineTest(t)
	ctx := context.Background()
	r, _ := store.Create(ctx, repo.CreateParams{
		Name: "stable", URL: "https://example.com/a.git", Branch: "main",
		Path: "/", PollInterval: "60s", Credential: "pat", AutoApply: true,
	})
	base := t.TempDir()
	repoDir := filepath.Join(base, r.ID)
	_ = os.MkdirAll(repoDir, 0o755)
	_ = os.WriteFile(filepath.Join(repoDir, "wf.yaml"), []byte(validWorkflowYAML("wf")), 0o644)

	d := &NoopDriver{BasePath: base, SHA: "first"}
	if _, err := SyncRepo(ctx, database, store, r, d); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	d.SHA = "second"
	report, err := SyncRepo(ctx, database, store, r, d)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if report.Applied != 0 || report.Unchanged != 1 {
		t.Errorf("second sync: applied=%d unchanged=%d, want 0/1", report.Applied, report.Unchanged)
	}
}

func TestSyncRepo_PrunesRemovedFiles(t *testing.T) {
	database, store := setupEngineTest(t)
	ctx := context.Background()
	r, _ := store.Create(ctx, repo.CreateParams{
		Name: "pru", URL: "https://example.com/a.git", Branch: "main",
		Path: "/", PollInterval: "60s", Credential: "pat",
		AutoApply: true, Prune: true,
	})
	base := t.TempDir()
	repoDir := filepath.Join(base, r.ID)
	_ = os.MkdirAll(repoDir, 0o755)

	// First sync: two workflows present.
	_ = os.WriteFile(filepath.Join(repoDir, "a.yaml"), []byte(validWorkflowYAML("a")), 0o644)
	_ = os.WriteFile(filepath.Join(repoDir, "b.yaml"), []byte(validWorkflowYAML("b")), 0o644)
	driver := &NoopDriver{BasePath: base, SHA: "first"}
	if _, err := SyncRepo(ctx, database, store, r, driver); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// Second sync: b.yaml is gone.
	_ = os.Remove(filepath.Join(repoDir, "b.yaml"))
	driver.SHA = "second"
	report, err := SyncRepo(ctx, database, store, r, driver)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if report.Pruned != 1 {
		t.Errorf("Pruned: got %d, want 1", report.Pruned)
	}

	// b should now be disabled (latest version has disabled_at IS NOT NULL).
	var disabledB sql.NullTime
	_ = database.QueryRowContext(ctx,
		`SELECT disabled_at FROM workflow_definitions WHERE name = 'b' ORDER BY version DESC LIMIT 1`,
	).Scan(&disabledB)
	if !disabledB.Valid {
		t.Error("workflow b should be disabled after prune")
	}

	// a should still be enabled.
	var disabledA sql.NullTime
	_ = database.QueryRowContext(ctx,
		`SELECT disabled_at FROM workflow_definitions WHERE name = 'a' ORDER BY version DESC LIMIT 1`,
	).Scan(&disabledA)
	if disabledA.Valid {
		t.Error("workflow a should stay enabled")
	}
}

func TestPlanRepo_ClassifiesFiles(t *testing.T) {
	database, store := setupEngineTest(t)
	ctx := context.Background()
	r, _ := store.Create(ctx, repo.CreateParams{
		Name: "plan", URL: "https://example.com/a.git", Branch: "main",
		Path: "/", PollInterval: "60s", Credential: "pat",
	})
	base := t.TempDir()
	repoDir := filepath.Join(base, r.ID)
	_ = os.MkdirAll(repoDir, 0o755)

	// Set up two workflows: both get applied in the priming sync.
	existingYAML := validWorkflowYAML("existing")
	_ = os.WriteFile(filepath.Join(repoDir, "existing.yaml"), []byte(existingYAML), 0o644)
	_ = os.WriteFile(filepath.Join(repoDir, "new.yaml"), []byte(validWorkflowYAML("new")), 0o644)

	// Prime the DB: apply both files so their hashes are recorded.
	driver := &NoopDriver{BasePath: base, SHA: "sha-before"}
	if _, err := SyncRepo(ctx, database, store, r, driver); err != nil {
		t.Fatalf("priming sync: %v", err)
	}

	// Now modify existing.yaml so its hash differs from what is in the DB.
	_ = os.WriteFile(filepath.Join(repoDir, "existing.yaml"),
		[]byte(existingYAML+"# changed\n"), 0o644)

	// Plan should see: existing changed → would apply (1), new unchanged (1).
	report, err := PlanRepo(ctx, database, r, driver)
	if err != nil {
		t.Fatalf("PlanRepo: %v", err)
	}
	if report.Applied != 1 || report.Unchanged != 1 {
		t.Errorf("PlanRepo: applied=%d unchanged=%d, want 1/1", report.Applied, report.Unchanged)
	}

	// Crucially: plan must NOT have written a new version of "existing".
	var count int
	_ = database.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM workflow_definitions WHERE name = 'existing'`,
	).Scan(&count)
	if count != 1 {
		t.Errorf("plan should not write: got %d versions of 'existing', want 1", count)
	}
}

func TestSyncRepo_InvalidFileDoesNotAbortOthers(t *testing.T) {
	database, store := setupEngineTest(t)
	ctx := context.Background()
	r, _ := store.Create(ctx, repo.CreateParams{
		Name: "mixed", URL: "https://example.com/a.git", Branch: "main",
		Path: "/", PollInterval: "60s", Credential: "pat", AutoApply: true,
	})
	base := t.TempDir()
	repoDir := filepath.Join(base, r.ID)
	_ = os.MkdirAll(repoDir, 0o755)
	_ = os.WriteFile(filepath.Join(repoDir, "good.yaml"), []byte(validWorkflowYAML("good")), 0o644)
	_ = os.WriteFile(filepath.Join(repoDir, "bad.yaml"), []byte("::: not yaml :::\n"), 0o644)

	d := &NoopDriver{BasePath: base, SHA: "mixed-sha"}
	report, err := SyncRepo(ctx, database, store, r, d)
	if err != nil {
		t.Fatalf("SyncRepo should not error on per-file failures: %v", err)
	}
	if report.Applied != 1 {
		t.Errorf("Applied: got %d, want 1", report.Applied)
	}
	if len(report.Failures) != 1 {
		t.Errorf("Failures: got %d, want 1", len(report.Failures))
	}
	got, _ := store.Get(ctx, "mixed")
	if got.LastSyncError == "" {
		t.Error("LastSyncError should describe the failed file")
	}
}
