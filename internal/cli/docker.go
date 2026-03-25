package cli

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/dvflw/mantle/internal/dbdefaults"
)

// dockerRunArgs returns the arguments for `docker run` to start a Postgres
// container matching Mantle's default configuration.
func dockerRunArgs() []string {
	return []string{
		"run", "-d",
		"--name", dbdefaults.ContainerName,
		"-p", "5432:5432",
		"-e", "POSTGRES_USER=" + dbdefaults.User,
		"-e", "POSTGRES_PASSWORD=" + dbdefaults.Password,
		"-e", "POSTGRES_DB=" + dbdefaults.Database,
		"-v", "mantle-pgdata:/var/lib/postgresql/data",
		dbdefaults.PostgresImage,
	}
}

// parseHostFromURL extracts the hostname from a Postgres connection URL.
func parseHostFromURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

// dockerAvailable checks whether the Docker CLI is installed and the daemon is responsive.
func dockerAvailable() bool {
	cmd := exec.Command("docker", "info")
	return cmd.Run() == nil
}

// dockerContainerStatus returns "running", "exited", or "" (not found)
// for the mantle-postgres container.
func dockerContainerStatus() string {
	out, err := exec.Command("docker", "inspect", "-f", "{{.State.Status}}", dbdefaults.ContainerName).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// dockerRemoveContainer removes the mantle-postgres container (stopped or otherwise).
func dockerRemoveContainer() error {
	return exec.Command("docker", "rm", "-f", dbdefaults.ContainerName).Run()
}

// dockerStartPostgres starts a new Postgres container and waits for it to accept connections.
func dockerStartPostgres(cfg config.DatabaseConfig) error {
	// Handle existing container.
	switch dockerContainerStatus() {
	case "running":
		// Already running — just wait for readiness.
		return waitForPostgres(cfg)
	case "exited", "created", "dead", "paused":
		_ = dockerRemoveContainer()
	}

	args := dockerRunArgs()
	out, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker run failed: %w\n%s", err, string(out))
	}

	return waitForPostgres(cfg)
}

// waitForPostgres polls db.Open with backoff until the database accepts connections
// or the timeout (~15s) is exceeded.
func waitForPostgres(cfg config.DatabaseConfig) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	delay := 500 * time.Millisecond
	for {
		database, err := db.Open(cfg)
		if err == nil {
			database.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("container started but Postgres isn't accepting connections after 15s: %w", err)
		case <-time.After(delay):
			if delay < 2*time.Second {
				delay *= 2
			}
		}
	}
}
