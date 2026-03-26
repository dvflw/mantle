package plugin

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// Plugin represents a running plugin subprocess.
type Plugin struct {
	Name    string
	Path    string
	Actions []string
	cancel  context.CancelFunc
}

// Manager manages plugin subprocess lifecycles.
type Manager struct {
	PluginDir string
	DB        *sql.DB
	Logger    *slog.Logger

	mu      sync.RWMutex
	plugins map[string]*Plugin
}

// NewManager creates a plugin manager that discovers plugins from the given directory.
func NewManager(pluginDir string, db *sql.DB) *Manager {
	return &Manager{
		PluginDir: pluginDir,
		DB:        db,
		Logger:    slog.Default(),
		plugins:   make(map[string]*Plugin),
	}
}

// Discover scans the plugin directory for executable plugins.
func (m *Manager) Discover() error {
	if m.PluginDir == "" {
		return nil
	}

	entries, err := os.ReadDir(m.PluginDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no plugin dir is fine
		}
		return fmt.Errorf("reading plugin directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(m.PluginDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		// Check if executable.
		if info.Mode()&0111 == 0 {
			continue
		}

		name := entry.Name()
		m.mu.Lock()
		m.plugins[name] = &Plugin{
			Name: name,
			Path: path,
		}
		m.mu.Unlock()
		m.Logger.Info("discovered plugin", "name", name, "path", path)
	}

	return nil
}

// List returns metadata for all discovered plugins.
func (m *Manager) List() []PluginInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	infos := make([]PluginInfo, 0, len(m.plugins))
	for _, p := range m.plugins {
		infos = append(infos, PluginInfo{
			Name:    p.Name,
			Path:    p.Path,
			Actions: p.Actions,
		})
	}
	return infos
}

// PluginInfo holds metadata about a discovered plugin.
type PluginInfo struct {
	Name    string   `json:"name"`
	Path    string   `json:"path"`
	Actions []string `json:"actions,omitempty"`
}

// Install copies or symlinks a plugin binary into the plugin directory.
func (m *Manager) Install(sourcePath string) error {
	if m.PluginDir == "" {
		return fmt.Errorf("plugin directory not configured (set plugins.dir in mantle.yaml)")
	}

	if err := os.MkdirAll(m.PluginDir, 0755); err != nil {
		return fmt.Errorf("creating plugin directory: %w", err)
	}

	name := filepath.Base(sourcePath)
	destPath := filepath.Join(m.PluginDir, name)

	// Copy the file.
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("reading plugin: %w", err)
	}
	if err := os.WriteFile(destPath, data, 0755); err != nil {
		return fmt.Errorf("writing plugin: %w", err)
	}

	m.mu.Lock()
	m.plugins[name] = &Plugin{Name: name, Path: destPath}
	m.mu.Unlock()

	m.Logger.Info("installed plugin", "name", name, "path", destPath)
	return nil
}

// Remove deletes a plugin from the plugin directory.
func (m *Manager) Remove(name string) error {
	m.mu.Lock()
	p, ok := m.plugins[name]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("plugin %q not found", name)
	}
	delete(m.plugins, name)
	m.mu.Unlock()

	if err := os.Remove(p.Path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing plugin file: %w", err)
	}

	m.Logger.Info("removed plugin", "name", name)
	return nil
}

// ExecutePlugin runs a plugin subprocess with the given action and params.
// The plugin receives input as JSON on stdin and writes output as JSON to stdout.
// This is a simpler protocol than gRPC for V1.1 — gRPC can be added later.
func (m *Manager) ExecutePlugin(ctx context.Context, name, action string, params map[string]any, credential map[string]string) (map[string]any, error) {
	m.mu.RLock()
	p, ok := m.plugins[name]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("plugin %q not found", name)
	}

	// Build input payload.
	input := map[string]any{
		"action":     action,
		"params":     params,
		"credential": credential,
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("marshaling input: %w", err)
	}

	// Run the plugin subprocess with a timeout.
	execCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, p.Path)
	cmd.Stdin = bytes(inputJSON)
	var stdout, stderr safeBuffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		stderrStr := stderr.String()
		if stderrStr != "" {
			return nil, fmt.Errorf("plugin %q failed: %s", name, stderrStr)
		}
		return nil, fmt.Errorf("plugin %q failed: %w", name, err)
	}

	// Parse output.
	var output map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		return nil, fmt.Errorf("parsing plugin output: %w (raw: %s)", err, stdout.String())
	}

	return output, nil
}

// Shutdown stops all running plugin processes.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.plugins {
		if p.cancel != nil {
			p.cancel()
		}
	}
}
