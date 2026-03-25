package connector

import (
	"fmt"
	"strings"
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
