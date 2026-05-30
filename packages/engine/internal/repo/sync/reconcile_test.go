package sync

import (
	"context"
	"testing"

	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/repo"
)

func TestReconcile_CreatesNewEntries(t *testing.T) {
	_, store := setupEngineTest(t)
	ctx := context.Background()

	cfg := []config.GitSyncRepo{
		{Name: "from-config", URL: "https://example.com/a.git", Branch: "main",
			Path: "/", PollInterval: "60s", Credential: "pat", AutoApply: true, Prune: true},
	}
	if err := Reconcile(ctx, store, cfg); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	got, err := store.Get(ctx, "from-config")
	if err != nil {
		t.Fatalf("Get after reconcile: %v", err)
	}
	if got.URL != "https://example.com/a.git" || got.Credential != "pat" {
		t.Errorf("reconciled repo wrong: %+v", got)
	}
}

func TestReconcile_UpdatesExistingByName(t *testing.T) {
	_, store := setupEngineTest(t)
	ctx := context.Background()
	_, err := store.Create(ctx, repo.CreateParams{
		Name: "shared", URL: "https://example.com/a.git", Branch: "main",
		Path: "/", PollInterval: "60s", Credential: "pat-v1", AutoApply: true, Prune: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	cfg := []config.GitSyncRepo{
		{Name: "shared", URL: "https://example.com/a.git", Branch: "release",
			Path: "/", PollInterval: "120s", Credential: "pat-v2", AutoApply: false, Prune: true},
	}
	if err := Reconcile(ctx, store, cfg); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	got, _ := store.Get(ctx, "shared")
	if got.Branch != "release" || got.Credential != "pat-v2" || got.AutoApply {
		t.Errorf("reconcile did not update fields: %+v", got)
	}
}

func TestReconcile_PreservesReposNotInConfig(t *testing.T) {
	_, store := setupEngineTest(t)
	ctx := context.Background()
	_, _ = store.Create(ctx, repo.CreateParams{
		Name: "cli-only", URL: "https://example.com/b.git", Branch: "main",
		Path: "/", PollInterval: "60s", Credential: "pat",
	})
	if err := Reconcile(ctx, store, nil); err != nil {
		t.Fatalf("Reconcile with empty config: %v", err)
	}
	if _, err := store.Get(ctx, "cli-only"); err != nil {
		t.Errorf("CLI-created repo should survive empty reconcile: %v", err)
	}
}
