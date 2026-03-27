package artifact

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFilesystemStorage_PutAndGet(t *testing.T) {
	dir := t.TempDir()
	fs := &FilesystemStorage{BasePath: dir}

	ctx := context.Background()
	key := "test-workflow/exec-123/my-artifact/data.tar.gz"
	data := []byte("fake tar content")

	// Write source file first.
	srcPath := filepath.Join(dir, "source.tar.gz")
	if err := os.WriteFile(srcPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	url, err := fs.Put(ctx, key, srcPath)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	if url == "" {
		t.Fatal("url should not be empty")
	}

	// Verify file exists at the stored location.
	stored, err := os.ReadFile(url)
	if err != nil {
		t.Fatalf("reading stored file: %v", err)
	}
	if string(stored) != string(data) {
		t.Errorf("stored content = %q, want %q", stored, data)
	}
}

func TestFilesystemStorage_Delete(t *testing.T) {
	dir := t.TempDir()
	fs := &FilesystemStorage{BasePath: dir}
	ctx := context.Background()

	// Write a file
	key := "test-workflow/exec-123/artifact/file.txt"
	srcPath := filepath.Join(dir, "src.txt")
	if err := os.WriteFile(srcPath, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	url, err := fs.Put(ctx, key, srcPath)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Delete by prefix
	err = fs.DeleteByPrefix(ctx, "test-workflow/exec-123/")
	if err != nil {
		t.Fatalf("DeleteByPrefix: %v", err)
	}

	// File should be gone
	if _, err := os.Stat(url); !os.IsNotExist(err) {
		t.Error("file should have been deleted")
	}
}

func TestFilesystemStorage_DeleteEscapeProtection(t *testing.T) {
	dir := t.TempDir()
	fs := &FilesystemStorage{BasePath: dir}
	ctx := context.Background()

	err := fs.DeleteByPrefix(ctx, "../../etc")
	if err == nil {
		t.Error("expected error for path traversal")
	}
}

func TestArtifactsDirContext(t *testing.T) {
	ctx := context.Background()

	// Empty by default
	if dir := ArtifactsDirFromContext(ctx); dir != "" {
		t.Errorf("expected empty, got %q", dir)
	}

	// Set and retrieve
	ctx = WithArtifactsDir(ctx, "/tmp/artifacts")
	if dir := ArtifactsDirFromContext(ctx); dir != "/tmp/artifacts" {
		t.Errorf("expected '/tmp/artifacts', got %q", dir)
	}
}
