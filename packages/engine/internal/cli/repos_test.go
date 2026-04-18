package cli

import (
	"bytes"
	"context"
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
// context with the DatabaseConfig attached so `newRepoStore` can load it.
func reposCtx(t *testing.T) (context.Context, *config.Config) {
	t.Helper()
	bg := context.Background()
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
	cfg := &config.Config{Database: dbCfg}
	ctx := config.WithContext(bg, cfg)
	return ctx, cfg
}

func TestReposAdd_PersistsRepo(t *testing.T) {
	_, cfg := reposCtx(t)
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
	_, cfg := reposCtx(t)
	// Seed one row by calling the add command.
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
	_, cfg := reposCtx(t)
	root := NewRootCommand()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"repos", "list", "--database-url", cfg.Database.URL})
	if err := root.Execute(); err != nil {
		t.Fatalf("list: %v\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "(no repos)") {
		t.Errorf("empty list output: %q", stdout.String())
	}
}

func TestReposStatus_ShowsDetails(t *testing.T) {
	_, cfg := reposCtx(t)
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
	_, cfg := reposCtx(t)
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
	_, cfg := reposCtx(t)
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
