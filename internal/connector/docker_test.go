package connector

import (
	"context"
	"strings"
	"testing"

	"github.com/docker/docker/client"
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
	if cfg.Remove {
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

func TestDockerRun_Integration_EchoStdout(t *testing.T) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer cli.Close()
	if _, err := cli.Ping(context.Background()); err != nil {
		t.Skipf("Docker not reachable: %v", err)
	}

	c := &DockerRunConnector{}
	output, err := c.Execute(context.Background(), map[string]any{
		"image": "alpine:latest",
		"cmd":   []any{"echo", "hello mantle"},
		"pull":  "missing",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if output["exit_code"] != int64(0) {
		t.Errorf("exit_code = %v, want 0", output["exit_code"])
	}
	stdout, _ := output["stdout"].(string)
	if !strings.Contains(stdout, "hello mantle") {
		t.Errorf("stdout = %q, want to contain 'hello mantle'", stdout)
	}
}

func TestDockerRun_Integration_NonZeroExitCode(t *testing.T) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer cli.Close()
	if _, err := cli.Ping(context.Background()); err != nil {
		t.Skipf("Docker not reachable: %v", err)
	}

	c := &DockerRunConnector{}
	output, err := c.Execute(context.Background(), map[string]any{
		"image": "alpine:latest",
		"cmd":   []any{"sh", "-c", "echo error-output >&2; exit 42"},
		"pull":  "missing",
	})
	// Non-zero exit code is NOT a step failure.
	if err != nil {
		t.Fatalf("Execute should not error on non-zero exit: %v", err)
	}
	if output["exit_code"] != int64(42) {
		t.Errorf("exit_code = %v, want 42", output["exit_code"])
	}
	stderr, _ := output["stderr"].(string)
	if !strings.Contains(stderr, "error-output") {
		t.Errorf("stderr = %q, want to contain 'error-output'", stderr)
	}
}

func TestDockerRun_Integration_Stdin(t *testing.T) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer cli.Close()
	if _, err := cli.Ping(context.Background()); err != nil {
		t.Skipf("Docker not reachable: %v", err)
	}

	c := &DockerRunConnector{}
	output, err := c.Execute(context.Background(), map[string]any{
		"image": "alpine:latest",
		"cmd":   []any{"cat"},
		"stdin": "piped input data",
		"pull":  "missing",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	stdout, _ := output["stdout"].(string)
	if !strings.Contains(stdout, "piped input data") {
		t.Errorf("stdout = %q, want to contain 'piped input data'", stdout)
	}
}

func TestDockerRun_Integration_EnvVars(t *testing.T) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer cli.Close()
	if _, err := cli.Ping(context.Background()); err != nil {
		t.Skipf("Docker not reachable: %v", err)
	}

	c := &DockerRunConnector{}
	output, err := c.Execute(context.Background(), map[string]any{
		"image": "alpine:latest",
		"cmd":   []any{"sh", "-c", "echo $MY_VAR"},
		"env":   map[string]any{"MY_VAR": "hello-from-env"},
		"pull":  "missing",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	stdout, _ := output["stdout"].(string)
	if !strings.Contains(stdout, "hello-from-env") {
		t.Errorf("stdout = %q, want 'hello-from-env'", stdout)
	}
}
