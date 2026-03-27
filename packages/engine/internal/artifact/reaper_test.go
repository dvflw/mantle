package artifact

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReaper_CleansExpiredArtifacts(t *testing.T) {
	dir := t.TempDir()
	fs := &FilesystemStorage{BasePath: dir}
	db := setupTestDB(t)
	store := &Store{DB: db}
	ctx := context.Background()
	execID := createTestExecution(t, db)

	// Write a file and create metadata
	srcPath := filepath.Join(dir, "src.bin")
	if err := os.WriteFile(srcPath, []byte("data"), 0644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	key := "wf/" + execID + "/artifact/file.bin"
	url, err := fs.Put(ctx, key, srcPath)
	if err != nil {
		t.Fatalf("fs.Put: %v", err)
	}

	if err := store.Create(ctx, &Artifact{
		ExecutionID: execID,
		StepName:    "step-a",
		Name:        "test-artifact",
		URL:         url,
		Size:        4,
	}); err != nil {
		t.Fatalf("store.Create: %v", err)
	}

	// Backdate the artifact so it appears expired
	_, err = db.ExecContext(ctx, `
		UPDATE execution_artifacts SET created_at = NOW() - interval '48 hours'
		WHERE execution_id = $1
	`, execID)
	if err != nil {
		t.Fatalf("backdate: %v", err)
	}

	reaper := &Reaper{
		Store:      store,
		Storage: fs,
		Retention:  24 * time.Hour,
	}

	cleaned, err := reaper.Sweep(ctx)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if cleaned != 1 {
		t.Errorf("cleaned = %d, want 1", cleaned)
	}

	// Metadata should be gone
	arts, listErr := store.ListByExecution(ctx, execID)
	if listErr != nil {
		t.Fatalf("ListByExecution: %v", listErr)
	}
	if len(arts) != 0 {
		t.Errorf("expected 0 artifacts after sweep, got %d", len(arts))
	}
}

func TestReaper_SkipsWhenRetentionZero(t *testing.T) {
	dir := t.TempDir()
	fs := &FilesystemStorage{BasePath: dir}

	reaper := &Reaper{
		Store:      &Store{}, // safe: Sweep returns early when Retention <= 0, before accessing Store.DB
		Storage: fs,
		Retention:  0,
	}

	cleaned, err := reaper.Sweep(context.Background())
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if cleaned != 0 {
		t.Errorf("cleaned = %d, want 0 when retention is 0", cleaned)
	}
}
