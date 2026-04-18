package sync

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dvflw/mantle/internal/repo"
)

func TestNoopDriver_Pull_ReturnsConfiguredPath(t *testing.T) {
	dir := t.TempDir()
	d := &NoopDriver{BasePath: dir, SHA: "abc123"}
	got, err := d.Pull(context.Background(), &repo.Repo{ID: "r1"})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	want := filepath.Join(dir, "r1")
	if got.LocalPath != want {
		t.Errorf("LocalPath: got %q, want %q", got.LocalPath, want)
	}
	if got.SHA != "abc123" {
		t.Errorf("SHA: got %q, want %q", got.SHA, "abc123")
	}
	if _, err := os.Stat(got.LocalPath); err != nil {
		t.Errorf("LocalPath does not exist: %v", err)
	}
}

func TestNoopDriver_Pull_CreatesDirectoryOnDemand(t *testing.T) {
	dir := t.TempDir()
	d := &NoopDriver{BasePath: dir}
	_, err := d.Pull(context.Background(), &repo.Repo{ID: "new-one"})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "new-one")); err != nil {
		t.Errorf("expected subdir to be created: %v", err)
	}
}
