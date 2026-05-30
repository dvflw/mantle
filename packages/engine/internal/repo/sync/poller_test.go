package sync

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/dvflw/mantle/internal/repo"
)

func TestPoller_TicksAndStops(t *testing.T) {
	database, store := setupEngineTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r, err := store.Create(ctx, repo.CreateParams{
		Name: "acme", URL: "https://example.com/a.git", Branch: "main",
		Path: "/", PollInterval: "10s", Credential: "pat",
		AutoApply: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	base := t.TempDir()
	repoDir := filepath.Join(base, r.ID)
	_ = os.MkdirAll(repoDir, 0o755)
	_ = os.WriteFile(filepath.Join(repoDir, "wf.yaml"), []byte(validWorkflowYAML("wf")), 0o644)

	var calls int
	var mu sync.Mutex
	driver := &NoopDriver{BasePath: base, SHA: "x"}
	p := &Poller{
		DB:     database,
		Store:  store,
		Driver: driver,
		OnSync: func(_ *repo.Repo, _ *Report, _ error) {
			mu.Lock()
			calls++
			mu.Unlock()
		},
		MinInterval: time.Millisecond,
	}
	go p.Run(ctx)

	time.Sleep(250 * time.Millisecond)
	cancel()

	mu.Lock()
	defer mu.Unlock()
	if calls < 2 {
		t.Errorf("expected >=2 sync calls, got %d", calls)
	}
}

func TestPoller_SkipsDisabledAndAutoApplyFalse(t *testing.T) {
	database, store := setupEngineTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := store.Create(ctx, repo.CreateParams{
		Name: "manual", URL: "https://example.com/a.git", Branch: "main",
		Path: "/", PollInterval: "10s", Credential: "pat",
		AutoApply: false,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var called bool
	var mu sync.Mutex
	p := &Poller{
		DB:    database,
		Store: store,
		Driver: &NoopDriver{BasePath: t.TempDir(), SHA: "x"},
		OnSync: func(_ *repo.Repo, _ *Report, _ error) {
			mu.Lock()
			called = true
			mu.Unlock()
		},
		MinInterval: time.Millisecond,
	}
	go p.Run(ctx)
	time.Sleep(150 * time.Millisecond)
	cancel()

	mu.Lock()
	defer mu.Unlock()
	if called {
		t.Error("poller synced an auto_apply:false repo")
	}
}
