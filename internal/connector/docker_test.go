package connector

import (
	"strings"
	"testing"
)

func TestDockerRun_MissingImage(t *testing.T) {
	_, err := parseDockerParams(map[string]any{
		"cmd": []any{"echo", "hello"},
	})
	if err == nil {
		t.Fatal("expected error for missing image")
	}
	if !strings.Contains(err.Error(), "image is required") {
		t.Errorf("error = %q, want 'image is required'", err)
	}
}

func TestDockerRun_ParseParams(t *testing.T) {
	params := map[string]any{
		"image":   "alpine:latest",
		"cmd":     []any{"echo", "hello"},
		"env":     map[string]any{"FOO": "bar"},
		"network": "host",
		"pull":    "never",
		"memory":  "256m",
		"cpus":    1.5,
		"remove":  false,
	}

	cfg, err := parseDockerParams(params)
	if err != nil {
		t.Fatalf("parseDockerParams: %v", err)
	}
	if cfg.Image != "alpine:latest" {
		t.Errorf("image = %q", cfg.Image)
	}
	if len(cfg.Cmd) != 2 || cfg.Cmd[0] != "echo" {
		t.Errorf("cmd = %v", cfg.Cmd)
	}
	if cfg.Env["FOO"] != "bar" {
		t.Errorf("env = %v", cfg.Env)
	}
	if cfg.Network != "host" {
		t.Errorf("network = %q", cfg.Network)
	}
	if cfg.Pull != "never" {
		t.Errorf("pull = %q", cfg.Pull)
	}
	if cfg.Memory != "256m" {
		t.Errorf("memory = %q", cfg.Memory)
	}
	if cfg.CPUs != 1.5 {
		t.Errorf("cpus = %f", cfg.CPUs)
	}
	if cfg.Remove != false {
		t.Errorf("remove = %v", cfg.Remove)
	}
}

func TestDockerRun_DefaultParams(t *testing.T) {
	cfg, err := parseDockerParams(map[string]any{
		"image": "alpine",
	})
	if err != nil {
		t.Fatalf("parseDockerParams: %v", err)
	}
	if cfg.Network != "bridge" {
		t.Errorf("default network = %q, want 'bridge'", cfg.Network)
	}
	if cfg.Pull != "missing" {
		t.Errorf("default pull = %q, want 'missing'", cfg.Pull)
	}
	if cfg.Remove != true {
		t.Errorf("default remove = %v, want true", cfg.Remove)
	}
}

func TestDockerRun_ParseMounts(t *testing.T) {
	cfg, err := parseDockerParams(map[string]any{
		"image": "alpine",
		"mounts": []any{
			map[string]any{
				"source":   "my-volume",
				"target":   "/data",
				"readonly": true,
			},
		},
	})
	if err != nil {
		t.Fatalf("parseDockerParams: %v", err)
	}
	if len(cfg.Mounts) != 1 {
		t.Fatalf("mounts = %d, want 1", len(cfg.Mounts))
	}
	if cfg.Mounts[0].Source != "my-volume" {
		t.Errorf("source = %q", cfg.Mounts[0].Source)
	}
	if cfg.Mounts[0].Target != "/data" {
		t.Errorf("target = %q", cfg.Mounts[0].Target)
	}
	if cfg.Mounts[0].ReadOnly != true {
		t.Errorf("readonly = %v", cfg.Mounts[0].ReadOnly)
	}
}

func TestParseMemoryString(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"256m", 256 * 1024 * 1024},
		{"1g", 1024 * 1024 * 1024},
		{"512k", 512 * 1024},
		{"1024", 1024},
		{"", 0},
	}
	for _, tt := range tests {
		got, err := parseMemoryString(tt.input)
		if err != nil {
			t.Errorf("parseMemoryString(%q): %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseMemoryString(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
