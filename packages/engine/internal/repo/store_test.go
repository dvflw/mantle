package repo

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/dvflw/mantle/internal/dbdefaults"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// setupTestDB spins up a Postgres container, runs migrations, and returns
// the connection. Copied from internal/environment/store_test.go to keep
// both packages decoupled from a shared test helper.
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
	t.Cleanup(func() { _ = pgContainer.Terminate(ctx) })
	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("ConnectionString: %v", err)
	}
	database, err := db.Open(config.DatabaseConfig{URL: connStr})
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(ctx, database); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}
	return database
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	return &Store{DB: setupTestDB(t), Actor: "test"}
}

// defaultCtx returns the default single-tenant test context. TeamIDFromContext
// returns auth.DefaultTeamID when no authenticated user is present, which
// matches the FK default on git_repos.
func defaultCtx() context.Context {
	return context.Background()
}

func TestStore_Create_PersistsRow(t *testing.T) {
	store := newTestStore(t)
	ctx := defaultCtx()

	r, err := store.Create(ctx, CreateParams{
		Name:         "acme",
		URL:          "https://github.com/acme/workflows.git",
		Branch:       "main",
		Path:         "/",
		PollInterval: "60s",
		Credential:   "github-pat",
		AutoApply:    true,
		Prune:        true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if r.ID == "" {
		t.Error("expected generated ID")
	}
	if r.Name != "acme" {
		t.Errorf("Name: got %q, want %q", r.Name, "acme")
	}
	if !r.Enabled {
		t.Error("Enabled should default to true")
	}
}

func TestStore_Create_RejectsDuplicateName(t *testing.T) {
	store := newTestStore(t)
	ctx := defaultCtx()
	base := CreateParams{
		Name:         "dup",
		URL:          "https://example.com/a.git",
		Branch:       "main",
		Path:         "/",
		PollInterval: "60s",
		Credential:   "pat",
	}
	if _, err := store.Create(ctx, base); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if _, err := store.Create(ctx, base); err == nil {
		t.Error("expected duplicate-name error")
	}
}

func TestStore_Create_ValidatesName(t *testing.T) {
	store := newTestStore(t)
	ctx := defaultCtx()
	_, err := store.Create(ctx, CreateParams{
		Name:         "Bad Name!",
		URL:          "https://example.com/a.git",
		Branch:       "main",
		Path:         "/",
		PollInterval: "60s",
		Credential:   "pat",
	})
	if err == nil {
		t.Error("expected name validation error")
	}
}

func TestStore_Create_ValidatesPollInterval(t *testing.T) {
	store := newTestStore(t)
	ctx := defaultCtx()
	_, err := store.Create(ctx, CreateParams{
		Name:         "slow",
		URL:          "https://example.com/a.git",
		Branch:       "main",
		Path:         "/",
		PollInterval: "5s",
		Credential:   "pat",
	})
	if err == nil {
		t.Error("expected poll_interval floor error")
	}
}

func TestStore_Get_ReturnsRow(t *testing.T) {
	store := newTestStore(t)
	ctx := defaultCtx()
	created, err := store.Create(ctx, CreateParams{
		Name:         "acme",
		URL:          "https://github.com/acme/workflows.git",
		Branch:       "main",
		Path:         "/",
		PollInterval: "60s",
		Credential:   "pat",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := store.Get(ctx, "acme")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID: got %q, want %q", got.ID, created.ID)
	}
}

func TestStore_Get_NotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := defaultCtx()
	_, err := store.Get(ctx, "nope")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestStore_List_ReturnsAllReposForTeam(t *testing.T) {
	store := newTestStore(t)
	ctx := defaultCtx()
	for _, name := range []string{"zeta", "alpha", "mike"} {
		if _, err := store.Create(ctx, CreateParams{
			Name:         name,
			URL:          "https://example.com/" + name + ".git",
			Branch:       "main",
			Path:         "/",
			PollInterval: "60s",
			Credential:   "pat",
		}); err != nil {
			t.Fatalf("Create %s: %v", name, err)
		}
	}
	repos, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(repos) != 3 {
		t.Fatalf("len: got %d, want 3", len(repos))
	}
	// ORDER BY name — alpha, mike, zeta.
	want := []string{"alpha", "mike", "zeta"}
	for i, r := range repos {
		if r.Name != want[i] {
			t.Errorf("index %d: got %q, want %q", i, r.Name, want[i])
		}
	}
}

func TestStore_Update_ReplacesMutableFields(t *testing.T) {
	store := newTestStore(t)
	ctx := defaultCtx()
	if _, err := store.Create(ctx, CreateParams{
		Name: "acme", URL: "https://example.com/a.git", Branch: "main",
		Path: "/", PollInterval: "60s", Credential: "pat", AutoApply: true, Prune: true,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	updated, err := store.Update(ctx, "acme", UpdateParams{
		Branch: "release", Path: "/workflows", PollInterval: "120s",
		Credential: "pat-v2", AutoApply: false, Prune: false, Enabled: true,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Branch != "release" || updated.Path != "/workflows" ||
		updated.PollInterval != "120s" || updated.Credential != "pat-v2" ||
		updated.AutoApply || updated.Prune {
		t.Errorf("Update did not persist fields: %+v", updated)
	}
}

func TestStore_Update_NotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := defaultCtx()
	_, err := store.Update(ctx, "nope", UpdateParams{
		Branch: "main", Path: "/", PollInterval: "60s", Credential: "pat",
	})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestStore_Delete_RemovesRow(t *testing.T) {
	store := newTestStore(t)
	ctx := defaultCtx()
	if _, err := store.Create(ctx, CreateParams{
		Name: "acme", URL: "https://example.com/a.git", Branch: "main",
		Path: "/", PollInterval: "60s", Credential: "pat",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Delete(ctx, "acme", ""); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get(ctx, "acme"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestStore_Delete_NotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := defaultCtx()
	if err := store.Delete(ctx, "nope", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestStore_Create_RejectsURLWithPassword(t *testing.T) {
	store := newTestStore(t)
	ctx := defaultCtx()
	_, err := store.Create(ctx, CreateParams{
		Name: "leaky", URL: "https://user:secret@github.com/acme/wf.git",
		Branch: "main", Path: "/", PollInterval: "60s", Credential: "pat",
	})
	if err == nil {
		t.Error("expected url-credential rejection")
	}
}

func TestStore_Create_AllowsURLWithUsernameOnly(t *testing.T) {
	store := newTestStore(t)
	ctx := defaultCtx()
	// GitHub HTTPS URLs often contain a username hint but no password — fine.
	if _, err := store.Create(ctx, CreateParams{
		Name: "ok", URL: "https://acme@github.com/acme/wf.git",
		Branch: "main", Path: "/", PollInterval: "60s", Credential: "pat",
	}); err != nil {
		t.Errorf("username-only URL should be allowed: %v", err)
	}
}

func TestStore_UpdateSyncState_WritesFields(t *testing.T) {
	store := newTestStore(t)
	ctx := defaultCtx()
	r, err := store.Create(ctx, CreateParams{
		Name: "acme", URL: "https://example.com/a.git", Branch: "main",
		Path: "/", PollInterval: "60s", Credential: "pat",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.UpdateSyncState(ctx, r.ID, "deadbeef", "pull failed"); err != nil {
		t.Fatalf("UpdateSyncState: %v", err)
	}
	got, err := store.Get(ctx, "acme")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.LastSyncSHA != "deadbeef" {
		t.Errorf("LastSyncSHA: got %q, want deadbeef", got.LastSyncSHA)
	}
	if got.LastSyncAt == nil {
		t.Error("LastSyncAt should be set")
	}
	if got.LastSyncError != "pull failed" {
		t.Errorf("LastSyncError: got %q, want %q", got.LastSyncError, "pull failed")
	}
}

func TestStore_GetByID_ReturnsRow(t *testing.T) {
	store := newTestStore(t)
	ctx := defaultCtx()
	created, _ := store.Create(ctx, CreateParams{
		Name: "acme", URL: "https://example.com/a.git", Branch: "main",
		Path: "/", PollInterval: "60s", Credential: "pat",
	})
	got, err := store.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "acme" {
		t.Errorf("Name: got %q, want acme", got.Name)
	}
}

func TestStore_GetByID_NotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := defaultCtx()
	_, err := store.GetByID(ctx, "00000000-0000-0000-0000-000000000000")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestStore_Delete_RemovesClonedDirectory(t *testing.T) {
	store := newTestStore(t)
	ctx := defaultCtx()
	r, err := store.Create(ctx, CreateParams{
		Name: "acme", URL: "https://example.com/a.git", Branch: "main",
		Path: "/", PollInterval: "60s", Credential: "pat",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	base := t.TempDir()
	repoDir := filepath.Join(base, r.ID)
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "wf.yaml"), []byte("name: wf\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := store.Delete(ctx, "acme", base); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(repoDir); !os.IsNotExist(err) {
		t.Errorf("clone dir should be gone: stat err = %v", err)
	}
}

func TestStore_Delete_IgnoresMissingCloneDir(t *testing.T) {
	store := newTestStore(t)
	ctx := defaultCtx()
	_, err := store.Create(ctx, CreateParams{
		Name: "gone", URL: "https://example.com/a.git", Branch: "main",
		Path: "/", PollInterval: "60s", Credential: "pat",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	base := t.TempDir()
	// NOTE: don't create the subdir — Delete should still succeed.
	if err := store.Delete(ctx, "gone", base); err != nil {
		t.Fatalf("Delete: missing clone dir caused failure: %v", err)
	}
}

func TestStore_UpdateSyncState_ClearsErrorWhenEmpty(t *testing.T) {
	store := newTestStore(t)
	ctx := defaultCtx()
	r, err := store.Create(ctx, CreateParams{
		Name: "clean", URL: "https://example.com/b.git", Branch: "main",
		Path: "/", PollInterval: "60s", Credential: "pat",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_ = store.UpdateSyncState(ctx, r.ID, "abc", "earlier error")
	if err := store.UpdateSyncState(ctx, r.ID, "def", ""); err != nil {
		t.Fatalf("UpdateSyncState: %v", err)
	}
	got, _ := store.Get(ctx, "clean")
	if got.LastSyncError != "" {
		t.Errorf("LastSyncError should clear to empty, got %q", got.LastSyncError)
	}
}
