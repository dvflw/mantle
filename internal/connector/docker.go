package connector

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
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
			if dm.Source == "" || dm.Target == "" {
				return nil, fmt.Errorf("docker/run: mount requires both source and target")
			}
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
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory value %q: %w", s, err)
	}
	if n < 0 {
		return 0, fmt.Errorf("negative memory value %q", s)
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

// Execute runs a Docker container to completion and returns its exit code, stdout, and stderr.
func (c *DockerRunConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	cfg, err := parseDockerParams(params)
	if err != nil {
		return nil, err
	}

	// Build client options from credential.
	var dockerHost string
	clientOpts := []client.Opt{client.WithAPIVersionNegotiation()}
	if cred, ok := params["_credential"].(map[string]string); ok {
		dockerHost = cred["host"]
		if dockerHost != "" {
			clientOpts = append(clientOpts, client.WithHost(dockerHost))
		}
		// TLS configuration.
		if cred["ca_cert"] != "" && cred["client_cert"] != "" && cred["client_key"] != "" {
			tmpDir, tmpErr := os.MkdirTemp("", "mantle-docker-tls-*")
			if tmpErr != nil {
				return nil, fmt.Errorf("docker/run: creating TLS temp dir: %w", tmpErr)
			}
			defer os.RemoveAll(tmpDir)
			caPath := filepath.Join(tmpDir, "ca.pem")
			certPath := filepath.Join(tmpDir, "cert.pem")
			keyPath := filepath.Join(tmpDir, "key.pem")
			if err := os.WriteFile(caPath, []byte(cred["ca_cert"]), 0600); err != nil {
				return nil, fmt.Errorf("docker/run: writing CA cert: %w", err)
			}
			if err := os.WriteFile(certPath, []byte(cred["client_cert"]), 0600); err != nil {
				return nil, fmt.Errorf("docker/run: writing client cert: %w", err)
			}
			if err := os.WriteFile(keyPath, []byte(cred["client_key"]), 0600); err != nil {
				return nil, fmt.Errorf("docker/run: writing client key: %w", err)
			}
			clientOpts = append(clientOpts, client.WithTLSClientConfig(caPath, certPath, keyPath))
		}
	} else {
		clientOpts = append(clientOpts, client.FromEnv)
	}

	cli, err := client.NewClientWithOpts(clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("docker/run: creating client: %w", err)
	}
	defer cli.Close()

	// Build registry auth for private image pulls.
	var registryAuth string
	if regCred, ok := params["_registry_credential"].(map[string]string); ok {
		authConfig := map[string]string{
			"username": regCred["username"],
			"password": regCred["password"],
		}
		authJSON, _ := json.Marshal(authConfig)
		registryAuth = base64.URLEncoding.EncodeToString(authJSON)
	}

	// Pull image.
	if err := pullImage(ctx, cli, cfg, registryAuth); err != nil {
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
		// Bind mounts only work with local Docker daemons.
		if dockerHost != "" && !strings.HasPrefix(dockerHost, "unix://") {
			return nil, fmt.Errorf("docker/run: artifact mounts are not supported with remote Docker daemons (host: %s); use s3/put inside the container instead", dockerHost)
		}
		hostCfg.Mounts = append(hostCfg.Mounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: artDir,
			Target: "/mantle/artifacts",
		})
	}

	// Resource limits.
	if cfg.Memory != "" {
		mem, parseErr := parseMemoryString(cfg.Memory)
		if parseErr != nil {
			return nil, fmt.Errorf("docker/run: invalid memory limit: %w", parseErr)
		}
		hostCfg.Resources.Memory = mem
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
			if _, err := io.Copy(attach.Conn, strings.NewReader(cfg.Stdin)); err != nil {
				log.Printf("docker/run: failed writing stdin to container: %v", err)
			}
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
	go func() { // #nosec G118 -- intentional context.Background: parent ctx is cancelled, need a live context to stop the container
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
		// Non-blocking check for a late error.
		select {
		case err := <-errCh:
			if err != nil {
				return nil, fmt.Errorf("docker/run: waiting for container: %w", err)
			}
		default:
		}
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
	if _, err := stdcopy.StdCopy(
		&limitWriter{w: &stdoutBuf, limit: DefaultMaxResponseBytes},
		&limitWriter{w: &stderrBuf, limit: DefaultMaxResponseBytes},
		logReader,
	); err != nil {
		log.Printf("docker/run: failed to demultiplex container logs: %v", err)
	}

	return map[string]any{
		"exit_code": exitCode,
		"stdout":    stdoutBuf.String(),
		"stderr":    stderrBuf.String(),
	}, nil
}

// pullImage handles image pulling based on the pull policy.
func pullImage(ctx context.Context, cli *client.Client, cfg *dockerConfig, registryAuth string) error {
	switch cfg.Pull {
	case "never":
		return nil
	case "always":
		// Always pull — fall through.
	case "missing":
		// Check if image exists locally.
		_, err := cli.ImageInspect(ctx, cfg.Image)
		if err == nil {
			return nil // already present
		}
	default:
		return fmt.Errorf("docker/run: invalid pull policy %q (must be always, missing, or never)", cfg.Pull)
	}

	reader, err := cli.ImagePull(ctx, cfg.Image, image.PullOptions{
		RegistryAuth: registryAuth,
	})
	if err != nil {
		return fmt.Errorf("docker/run: pulling image %q: %w", cfg.Image, err)
	}
	defer reader.Close()
	// Drain the pull output to complete the pull.
	if _, err := io.Copy(io.Discard, reader); err != nil {
		return fmt.Errorf("docker/run: draining pull output: %w", err)
	}
	return nil
}
