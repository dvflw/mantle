package connector

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/dvflw/mantle/internal/artifact"
)

// DockerRunConnector runs a container to completion and captures output.
type DockerRunConnector struct{}

type dockerConfig struct {
	Image   string
	Cmd     []string
	Env     map[string]string
	Stdin   string
	Mounts  []dockerMount
	Network string
	Pull    string // "always", "missing", "never"
	Memory  string
	CPUs    float64
	Remove  bool
}

type dockerMount struct {
	Source   string
	Target   string
	ReadOnly bool
}

func parseDockerParams(params map[string]any) (*dockerConfig, error) {
	img, _ := params["image"].(string)
	if img == "" {
		return nil, fmt.Errorf("docker/run: image is required")
	}

	cfg := &dockerConfig{
		Image:   img,
		Network: "bridge",
		Pull:    "missing",
		Remove:  true,
	}

	// cmd
	if raw, ok := params["cmd"].([]any); ok {
		for _, v := range raw {
			if s, ok := v.(string); ok {
				cfg.Cmd = append(cfg.Cmd, s)
			}
		}
	}

	// env
	if raw, ok := params["env"].(map[string]any); ok {
		cfg.Env = make(map[string]string, len(raw))
		for k, v := range raw {
			cfg.Env[k] = fmt.Sprintf("%v", v)
		}
	}

	// stdin
	cfg.Stdin, _ = params["stdin"].(string)

	// mounts
	if raw, ok := params["mounts"].([]any); ok {
		for _, item := range raw {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			dm := dockerMount{}
			dm.Source, _ = m["source"].(string)
			dm.Target, _ = m["target"].(string)
			dm.ReadOnly, _ = m["readonly"].(bool)
			cfg.Mounts = append(cfg.Mounts, dm)
		}
	}

	// network
	if n, ok := params["network"].(string); ok && n != "" {
		cfg.Network = n
	}

	// pull
	if p, ok := params["pull"].(string); ok && p != "" {
		cfg.Pull = p
	}

	// memory
	cfg.Memory, _ = params["memory"].(string)

	// cpus
	switch v := params["cpus"].(type) {
	case float64:
		cfg.CPUs = v
	case int:
		cfg.CPUs = float64(v)
	case int64:
		cfg.CPUs = float64(v)
	}

	// remove
	if r, ok := params["remove"].(bool); ok {
		cfg.Remove = r
	}

	return cfg, nil
}

// parseMemoryString converts strings like "256m", "1g" to bytes.
func parseMemoryString(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, nil
	}
	var multiplier int64 = 1
	switch {
	case strings.HasSuffix(s, "g"):
		multiplier = 1024 * 1024 * 1024
		s = s[:len(s)-1]
	case strings.HasSuffix(s, "m"):
		multiplier = 1024 * 1024
		s = s[:len(s)-1]
	case strings.HasSuffix(s, "k"):
		multiplier = 1024
		s = s[:len(s)-1]
	}
	var n int64
	_, err := fmt.Sscanf(s, "%d", &n)
	if err != nil {
		return 0, fmt.Errorf("invalid memory value %q", s)
	}
	return n * multiplier, nil
}

// limitWriter wraps a writer to limit bytes written. Logs a warning on first truncation.
type limitWriter struct {
	w         io.Writer
	limit     int64
	n         int64
	truncated bool
}

func (lw *limitWriter) Write(p []byte) (int, error) {
	remaining := lw.limit - lw.n
	if remaining <= 0 {
		if !lw.truncated {
			lw.truncated = true
			log.Printf("docker/run: output truncated at %d bytes", lw.limit)
		}
		return len(p), nil
	}
	if int64(len(p)) > remaining {
		p = p[:remaining]
		lw.truncated = true
		log.Printf("docker/run: output truncated at %d bytes", lw.limit)
	}
	n, err := lw.w.Write(p)
	lw.n += int64(n)
	return n, err
}

func (c *DockerRunConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	cfg, err := parseDockerParams(params)
	if err != nil {
		return nil, err
	}

	// Build client options from credential.
	clientOpts := []client.Opt{client.WithAPIVersionNegotiation()}
	if cred, ok := params["_credential"].(map[string]string); ok {
		delete(params, "_credential")
		if host := cred["host"]; host != "" {
			clientOpts = append(clientOpts, client.WithHost(host))
		}
	} else {
		delete(params, "_credential")
		clientOpts = append(clientOpts, client.FromEnv)
	}

	cli, err := client.NewClientWithOpts(clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("docker/run: creating client: %w", err)
	}
	defer cli.Close()

	// Pull image.
	if err := pullImage(ctx, cli, cfg); err != nil {
		return nil, err
	}

	// Build container config.
	containerCfg := &container.Config{
		Image: cfg.Image,
		Cmd:   cfg.Cmd,
	}

	// Env vars.
	for k, v := range cfg.Env {
		containerCfg.Env = append(containerCfg.Env, k+"="+v)
	}

	// Stdin.
	if cfg.Stdin != "" {
		containerCfg.OpenStdin = true
		containerCfg.StdinOnce = true
		containerCfg.AttachStdin = true
	}

	// Host config (mounts, resources).
	hostCfg := &container.HostConfig{
		NetworkMode: container.NetworkMode(cfg.Network),
	}

	// Mounts from params.
	for _, m := range cfg.Mounts {
		hostCfg.Mounts = append(hostCfg.Mounts, mount.Mount{
			Type:     mount.TypeVolume,
			Source:   m.Source,
			Target:   m.Target,
			ReadOnly: m.ReadOnly,
		})
	}

	// Artifacts dir mount (from context).
	if artDir := artifact.ArtifactsDirFromContext(ctx); artDir != "" {
		hostCfg.Mounts = append(hostCfg.Mounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: artDir,
			Target: "/mantle/artifacts",
		})
	}

	// Resource limits.
	if cfg.Memory != "" {
		mem, parseErr := parseMemoryString(cfg.Memory)
		if parseErr == nil {
			hostCfg.Resources.Memory = mem
		}
	}
	if cfg.CPUs > 0 {
		hostCfg.Resources.NanoCPUs = int64(cfg.CPUs * 1e9)
	}

	// Create container.
	resp, err := cli.ContainerCreate(ctx, containerCfg, hostCfg, &network.NetworkingConfig{}, nil, "")
	if err != nil {
		return nil, fmt.Errorf("docker/run: creating container: %w", err)
	}
	containerID := resp.ID

	// Ensure cleanup.
	defer func() {
		if cfg.Remove {
			cli.ContainerRemove(context.Background(), containerID, container.RemoveOptions{Force: true})
		}
	}()

	// Attach for stdin if needed.
	if cfg.Stdin != "" {
		attach, attachErr := cli.ContainerAttach(ctx, containerID, container.AttachOptions{
			Stream: true,
			Stdin:  true,
		})
		if attachErr != nil {
			return nil, fmt.Errorf("docker/run: attaching stdin: %w", attachErr)
		}
		go func() {
			defer attach.Close()
			io.Copy(attach.Conn, strings.NewReader(cfg.Stdin))
			attach.CloseWrite()
		}()
	}

	// Start container.
	if err := cli.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("docker/run: starting container: %w", err)
	}

	// Handle context cancellation → container stop.
	doneCh := make(chan struct{})
	defer close(doneCh)
	go func() {
		select {
		case <-ctx.Done():
			gracePeriod := 10
			cli.ContainerStop(context.Background(), containerID, container.StopOptions{
				Timeout: &gracePeriod,
			})
		case <-doneCh:
		}
	}()

	// Wait for container to finish.
	statusCh, errCh := cli.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
	var exitCode int64
	select {
	case err := <-errCh:
		if err != nil {
			return nil, fmt.Errorf("docker/run: waiting for container: %w", err)
		}
	case status := <-statusCh:
		exitCode = status.StatusCode
	}

	// Capture logs (stdout + stderr).
	logReader, err := cli.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		return nil, fmt.Errorf("docker/run: reading logs: %w", err)
	}
	defer logReader.Close()

	var stdoutBuf, stderrBuf strings.Builder
	stdcopy.StdCopy(
		&limitWriter{w: &stdoutBuf, limit: DefaultMaxResponseBytes},
		&limitWriter{w: &stderrBuf, limit: DefaultMaxResponseBytes},
		logReader,
	)

	return map[string]any{
		"exit_code": exitCode,
		"stdout":    stdoutBuf.String(),
		"stderr":    stderrBuf.String(),
	}, nil
}

// pullImage handles image pulling based on the pull policy.
func pullImage(ctx context.Context, cli *client.Client, cfg *dockerConfig) error {
	switch cfg.Pull {
	case "never":
		return nil
	case "always":
		// Always pull — fall through.
	case "missing":
		// Check if image exists locally.
		_, _, err := cli.ImageInspectWithRaw(ctx, cfg.Image)
		if err == nil {
			return nil // already present
		}
	default:
		return fmt.Errorf("docker/run: invalid pull policy %q (must be always, missing, or never)", cfg.Pull)
	}

	reader, err := cli.ImagePull(ctx, cfg.Image, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("docker/run: pulling image %q: %w", cfg.Image, err)
	}
	defer reader.Close()
	// Drain the pull output to complete the pull.
	io.Copy(io.Discard, reader)
	return nil
}
