package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManager_Discover_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir, nil)

	err := mgr.Discover()
	if err != nil {
		t.Fatalf("Discover() error: %v", err)
	}

	plugins := mgr.List()
	if len(plugins) != 0 {
		t.Errorf("List() returned %d, want 0", len(plugins))
	}
}

func TestManager_Discover_NoDir(t *testing.T) {
	mgr := NewManager("/nonexistent/path", nil)
	err := mgr.Discover()
	if err != nil {
		t.Fatalf("Discover() should not error for missing dir, got: %v", err)
	}
}

func TestManager_Discover_FindsExecutables(t *testing.T) {
	dir := t.TempDir()

	// Create an executable file.
	path := filepath.Join(dir, "my-plugin")
	os.WriteFile(path, []byte("#!/bin/sh\necho {}"), 0755)

	// Create a non-executable file.
	os.WriteFile(filepath.Join(dir, "not-a-plugin.txt"), []byte("data"), 0644)

	mgr := NewManager(dir, nil)
	err := mgr.Discover()
	if err != nil {
		t.Fatalf("Discover() error: %v", err)
	}

	plugins := mgr.List()
	if len(plugins) != 1 {
		t.Fatalf("List() returned %d, want 1", len(plugins))
	}
	if plugins[0].Name != "my-plugin" {
		t.Errorf("name = %q, want %q", plugins[0].Name, "my-plugin")
	}
}

func TestManager_InstallAndRemove(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir, nil)

	// Create a source plugin file.
	src := filepath.Join(t.TempDir(), "test-plugin")
	os.WriteFile(src, []byte("#!/bin/sh\necho {}"), 0755)

	// Install.
	err := mgr.Install(src)
	if err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	plugins := mgr.List()
	if len(plugins) != 1 {
		t.Fatalf("List() after install = %d, want 1", len(plugins))
	}

	// Verify file exists in plugin dir.
	destPath := filepath.Join(dir, "test-plugin")
	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		t.Error("plugin file not found in plugin directory")
	}

	// Remove.
	err = mgr.Remove("test-plugin")
	if err != nil {
		t.Fatalf("Remove() error: %v", err)
	}

	plugins = mgr.List()
	if len(plugins) != 0 {
		t.Errorf("List() after remove = %d, want 0", len(plugins))
	}
}

func TestManager_Remove_NotFound(t *testing.T) {
	mgr := NewManager(t.TempDir(), nil)
	err := mgr.Remove("nonexistent")
	if err == nil {
		t.Error("Remove() should error for nonexistent plugin")
	}
}

func TestManager_ExecutePlugin(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir, nil)

	// Create a simple plugin that reads stdin JSON and outputs a response.
	script := `#!/bin/sh
echo '{"result":"hello from plugin"}'
`
	path := filepath.Join(dir, "echo-plugin")
	os.WriteFile(path, []byte(script), 0755)

	mgr.Discover()

	output, err := mgr.ExecutePlugin(t.Context(), "echo-plugin", "test", map[string]any{"key": "value"}, nil)
	if err != nil {
		t.Fatalf("ExecutePlugin() error: %v", err)
	}

	if output["result"] != "hello from plugin" {
		t.Errorf("result = %v, want %q", output["result"], "hello from plugin")
	}
}
