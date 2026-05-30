package sync

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscover_CollectsYAMLFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.yaml", "name: alpha\nsteps: []\n")
	writeFile(t, dir, "b.yml", "name: beta\nsteps: []\n")
	writeFile(t, dir, "readme.md", "not yaml")
	writeFile(t, dir, "nested/c.yaml", "name: gamma\nsteps: []\n")

	found, err := Discover(dir, "/")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(found) != 3 {
		t.Fatalf("got %d files, want 3: %+v", len(found), found)
	}
	names := map[string]bool{}
	for _, f := range found {
		names[filepath.Base(f.RelPath)] = true
		if f.Hash == "" {
			t.Errorf("empty hash on %s", f.RelPath)
		}
		if len(f.Bytes) == 0 {
			t.Errorf("empty bytes on %s", f.RelPath)
		}
	}
	for _, want := range []string{"a.yaml", "b.yml", "c.yaml"} {
		if !names[want] {
			t.Errorf("missing %s in results", want)
		}
	}
}

func TestDiscover_RespectsSubPath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "root.yaml", "name: root\n")
	writeFile(t, dir, "workflows/a.yaml", "name: a\n")
	writeFile(t, dir, "workflows/b.yaml", "name: b\n")

	found, err := Discover(dir, "/workflows")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(found) != 2 {
		t.Fatalf("got %d, want 2", len(found))
	}
	for _, f := range found {
		if filepath.Base(f.RelPath) == "root.yaml" {
			t.Errorf("root.yaml should not be discovered with path=/workflows")
		}
	}
}

func TestDiscover_MissingPath(t *testing.T) {
	dir := t.TempDir()
	_, err := Discover(dir, "/does-not-exist")
	if err == nil {
		t.Error("expected error for missing subpath")
	}
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", full, err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
}
