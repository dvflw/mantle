package cli

import (
	"bytes"
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

// reposCtx spins up a Postgres container, runs migrations, and returns a
// *config.Config with the DatabaseConfig attached. Callers pass the database
// URL via --database-url flags; the config is used only to read that URL.
func reposCtx(t *testing.T) *config.Config {
	t.Helper()
	bg := t.Context()
	pgContainer, err := postgres.Run(bg,
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
	t.Cleanup(func() { _ = pgContainer.Terminate(bg) })
	connStr, err := pgContainer.ConnectionString(bg, "sslmode=disable")
	if err != nil {
		t.Fatalf("ConnectionString: %v", err)
	}
	dbCfg := config.DatabaseConfig{URL: connStr}
	conn, err := db.Open(dbCfg)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(bg, conn); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}
	return &config.Config{Database: dbCfg}
}

// seedRepo registers a single repo via the add command, failing the test on
// error. Shared by tests that need at least one row in git_repos.
func seedRepo(t *testing.T, cfg *config.Config, name string) {
	t.Helper()
	root := NewRootCommand()
	var seedStderr bytes.Buffer
	root.SetErr(&seedStderr)
	root.SetArgs([]string{"repos", "add", name,
		"--url", "https://example.com/a.git",
		"--credential", "pat",
		"--database-url", cfg.Database.URL,
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("seedRepo(%q): %v\nstderr: %s", name, err, seedStderr.String())
	}
}

func TestReposAdd_PersistsRepo(t *testing.T) {
	cfg := reposCtx(t)
	root := NewRootCommand()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"repos", "add", "acme",
		"--url", "https://github.com/acme/workflows.git",
		"--credential", "github-pat",
		"--database-url", cfg.Database.URL,
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Added repo \"acme\"") {
		t.Errorf("unexpected stdout: %q", stdout.String())
	}
}

func TestReposList_ShowsRegisteredRepos(t *testing.T) {
	cfg := reposCtx(t)
	seedRepo(t, cfg, "acme")

	root := NewRootCommand()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"repos", "list", "--database-url", cfg.Database.URL})
	if err := root.Execute(); err != nil {
		t.Fatalf("list: %v\nstderr: %s", err, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "acme") || !strings.Contains(out, "NAME") {
		t.Errorf("list output missing expected fields: %q", out)
	}
}

func TestReposList_EmptyState(t *testing.T) {
	cfg := reposCtx(t)
	root := NewRootCommand()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"repos", "list", "--database-url", cfg.Database.URL})
	if err := root.Execute(); err != nil {
		t.Fatalf("list: %v\nstderr: %s", err, stderr.String())
	}
	if strings.TrimSpace(stdout.String()) != "(no repos)" {
		t.Errorf("empty list output: %q", stdout.String())
	}
}

func TestReposStatus_ShowsDetails(t *testing.T) {
	cfg := reposCtx(t)
	seedRepo(t, cfg, "acme")

	root := NewRootCommand()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"repos", "status", "acme", "--database-url", cfg.Database.URL})
	if err := root.Execute(); err != nil {
		t.Fatalf("status: %v\nstderr: %s", err, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"Name:", "URL:", "Branch:", "Credential:", "Auto-Apply:"} {
		if !strings.Contains(out, want) {
			t.Errorf("status output missing %q: %s", want, out)
		}
	}
}

func TestReposRemove_RequiresYesFlag(t *testing.T) {
	cfg := reposCtx(t)
	seedRepo(t, cfg, "acme")

	root := NewRootCommand()
	var stderr bytes.Buffer
	root.SetErr(&stderr)
	root.SetArgs([]string{"repos", "remove", "acme", "--database-url", cfg.Database.URL})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Errorf("expected --yes error, got %v", err)
	}
}

func TestReposRemove_DeletesRow(t *testing.T) {
	cfg := reposCtx(t)
	seedRepo(t, cfg, "acme")

	root := NewRootCommand()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"repos", "remove", "acme", "--yes", "--database-url", cfg.Database.URL})
	if err := root.Execute(); err != nil {
		t.Fatalf("remove: %v\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Removed repo \"acme\"") {
		t.Errorf("unexpected stdout: %q", stdout.String())
	}
}

func TestReposSync_UsesNoopDriverWithFromDir(t *testing.T) {
	cfg := reposCtx(t)
	// Seed a repo.
	{
		root := NewRootCommand()
		var seedStderr bytes.Buffer
		root.SetErr(&seedStderr)
		root.SetArgs([]string{"repos", "add", "acme",
			"--url", "https://example.com/a.git",
			"--credential", "pat",
			"--database-url", cfg.Database.URL,
		})
		if err := root.Execute(); err != nil {
			t.Fatalf("seed add: %v\nstderr: %s", err, seedStderr.String())
		}
	}

	// Pre-populate a fixture dir. NoopDriver creates BasePath/<repo.ID>/
	// and the sync engine walks r.Path (default "/") inside that dir.
	// An empty dir yields Applied=0 Unchanged=0 — valid outcome.
	fixture := t.TempDir()

	root := NewRootCommand()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"repos", "sync", "acme",
		"--from-dir", fixture,
		"--database-url", cfg.Database.URL,
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync: %v\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Synced acme") {
		t.Errorf("expected 'Synced acme' in stdout, got %q", stdout.String())
	}
}
