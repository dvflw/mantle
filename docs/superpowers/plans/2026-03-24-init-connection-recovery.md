# `mantle init` Connection Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `mantle init` handle missing Postgres gracefully — auto-provision via Docker on localhost, retry/quit on remote hosts — and update the quickstart docs to match.

**Architecture:** When `db.Open` fails, classify the host as loopback or remote. Loopback failures offer Docker auto-provisioning; remote failures offer retry/quit. Extract duplicated constants (testcontainers defaults, loopback detection, budget modes) into shared packages first.

**Tech Stack:** Go, Cobra (`cmd.InOrStdin()`/`cmd.OutOrStdout()`), `os/exec` for Docker commands, `net/url` + `net` for host parsing.

**Spec:** `docs/superpowers/specs/2026-03-24-init-connection-recovery-design.md`

---

### Task 1: Create `internal/netutil/loopback.go` — loopback detection

**Files:**
- Create: `internal/netutil/loopback.go`
- Create: `internal/netutil/loopback_test.go`

- [ ] **Step 1: Write the failing tests**

In `internal/netutil/loopback_test.go`:

```go
package netutil_test

import (
	"testing"

	"github.com/dvflw/mantle/internal/netutil"
	"github.com/stretchr/testify/assert"
)

func TestIsLoopback(t *testing.T) {
	tests := []struct {
		host     string
		expected bool
	}{
		{"localhost", true},
		{"LOCALHOST", true},
		{"Localhost", true},
		{"127.0.0.1", true},
		{"::1", true},
		{"db.example.com", false},
		{"10.0.0.1", false},
		{"192.168.1.1", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			assert.Equal(t, tt.expected, netutil.IsLoopback(tt.host))
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/netutil/ -v`
Expected: FAIL — package does not exist yet

- [ ] **Step 3: Write minimal implementation**

In `internal/netutil/loopback.go`:

```go
package netutil

import (
	"net"
	"strings"
)

// IsLoopback returns true if host is a loopback address: localhost, 127.0.0.1, or ::1.
func IsLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/netutil/ -v`
Expected: PASS — all 9 cases

- [ ] **Step 5: Commit**

```bash
git add internal/netutil/loopback.go internal/netutil/loopback_test.go
git commit -m "feat(netutil): add IsLoopback host classifier"
```

---

### Task 2: Adopt `netutil.IsLoopback` in `internal/config/config.go`

**Files:**
- Modify: `internal/config/config.go:268-280` (SSL warning block)

- [ ] **Step 1: Run existing config tests as baseline**

Run: `go test ./internal/config/ -v`
Expected: PASS — all existing tests green

- [ ] **Step 2: Replace inline loopback logic with `netutil.IsLoopback`**

In `internal/config/config.go`, replace the SSL warning block (lines ~268-281):

```go
// Current code:
	if dbURL := cfg.Database.URL; dbURL != "" {
		if parsed, err := url.Parse(dbURL); err == nil {
			host := parsed.Hostname()
			ip := net.ParseIP(host)
			isLoopback := host != "" && (strings.EqualFold(host, "localhost") || (ip != nil && ip.IsLoopback()))
			if !isLoopback {
				q := parsed.Query()
				if q.Get("sslmode") == "prefer" {
					log.Printf("WARNING: database URL uses sslmode=prefer for non-loopback host %q; consider sslmode=require for production", host)
				}
			}
		}
	}
```

Replace with:

```go
	if dbURL := cfg.Database.URL; dbURL != "" {
		if parsed, err := url.Parse(dbURL); err == nil {
			host := parsed.Hostname()
			if !netutil.IsLoopback(host) {
				q := parsed.Query()
				if q.Get("sslmode") == "prefer" {
					log.Printf("WARNING: database URL uses sslmode=prefer for non-loopback host %q; consider sslmode=require for production", host)
				}
			}
		}
	}
```

Add import `"github.com/dvflw/mantle/internal/netutil"`. Remove `"net"` from imports if no longer used (check — `net` may be used elsewhere in the file). Remove `"strings"` only if no longer used elsewhere.

- [ ] **Step 3: Run config tests to verify no regression**

Run: `go test ./internal/config/ -v`
Expected: PASS — identical behavior

- [ ] **Step 4: Commit**

```bash
git add internal/config/config.go
git commit -m "refactor(config): use netutil.IsLoopback for SSL warning"
```

---

### Task 3: Create `internal/dbdefaults/dbdefaults.go` — shared constants

**Files:**
- Create: `internal/dbdefaults/dbdefaults.go`

- [ ] **Step 1: Create the constants package**

In `internal/dbdefaults/dbdefaults.go`:

```go
package dbdefaults

// Runtime defaults — used by Docker auto-provisioning and config defaults.
// These match the default database URL in config.go.
const (
	PostgresImage = "postgres:16-alpine"
	User          = "mantle"
	Password      = "mantle"
	Database      = "mantle"
	ContainerName = "mantle-postgres"
)

// Test defaults — used by testcontainers setups.
const (
	TestDatabase = "mantle_test"
)
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/dbdefaults/`
Expected: success (no output)

- [ ] **Step 3: Commit**

```bash
git add internal/dbdefaults/dbdefaults.go
git commit -m "feat(dbdefaults): add shared Postgres image and test credential constants"
```

---

### Task 4: Adopt `dbdefaults` in all testcontainers setups

**Files:**
- Modify: `internal/db/migrate_test.go:19-22`
- Modify: `internal/budget/store_test.go:23-26`
- Modify: `internal/engine/test_helpers_test.go:21-24`
- Modify: `internal/auth/auth_test.go` (find `setupTestDB`)
- Modify: `internal/workflow/store_test.go` (find `setupTestDB`)
- Modify: `internal/secret/store_test.go` (find `setupTestDB`)
- Modify: `internal/connector/postgres_test.go` (find postgres image literal)

- [ ] **Step 1: Run all tests as baseline**

Run: `go test ./internal/db/ ./internal/budget/ ./internal/engine/ ./internal/auth/ ./internal/workflow/ ./internal/secret/ ./internal/connector/ -count=1 -short`
Expected: PASS (or SKIP if Docker not available)

- [ ] **Step 2: Update each test file**

In each file's `setupTestDB` function, replace the string literals with `dbdefaults` constants. The pattern is the same in every file. Replace:

```go
	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("mantle_test"),
		postgres.WithUsername("mantle"),
		postgres.WithPassword("mantle"),
```

With:

```go
	pgContainer, err := postgres.Run(ctx,
		dbdefaults.PostgresImage,
		postgres.WithDatabase(dbdefaults.TestDatabase),
		postgres.WithUsername(dbdefaults.User),
		postgres.WithPassword(dbdefaults.Password),
```

Add import `"github.com/dvflw/mantle/internal/dbdefaults"` to each file.

Files to update (7 total):
1. `internal/db/migrate_test.go`
2. `internal/budget/store_test.go`
3. `internal/engine/test_helpers_test.go`
4. `internal/auth/auth_test.go`
5. `internal/workflow/store_test.go`
6. `internal/secret/store_test.go`
7. `internal/connector/postgres_test.go` (only `PostgresImage` — check if it uses different user/db)

- [ ] **Step 3: Verify compilation**

Run: `go build ./internal/...`
Expected: success

- [ ] **Step 4: Run tests to verify no regression**

Run: `go test ./internal/db/ ./internal/budget/ ./internal/engine/ ./internal/auth/ ./internal/workflow/ ./internal/secret/ ./internal/connector/ -count=1 -short`
Expected: same results as baseline

- [ ] **Step 5: Commit**

```bash
git add internal/db/migrate_test.go internal/budget/store_test.go internal/engine/test_helpers_test.go internal/auth/auth_test.go internal/workflow/store_test.go internal/secret/store_test.go internal/connector/postgres_test.go
git commit -m "refactor(tests): use dbdefaults constants in all testcontainers setups"
```

---

### Task 5: Add budget reset mode constants

**Files:**
- Modify: `internal/budget/budget.go:1-22`
- Modify: `internal/config/config.go:260-261`

- [ ] **Step 1: Run baseline tests**

Run: `go test ./internal/budget/ ./internal/config/ -v`
Expected: PASS

- [ ] **Step 2: Add constants to budget.go**

At the top of `internal/budget/budget.go`, after the imports, add:

```go
// Reset mode constants for budget period calculation.
const (
	ResetModeCalendar = "calendar"
	ResetModeRolling  = "rolling"
)
```

Update `CurrentPeriodStart` to use the constant:

```go
func CurrentPeriodStart(now time.Time, mode string, resetDay int) time.Time {
	now = now.UTC()
	if mode == ResetModeRolling && resetDay >= 1 && resetDay <= 28 {
```

- [ ] **Step 3: Update config.go validation to use budget constants**

In `internal/config/config.go`, replace the string literals in validation (line ~261):

```go
// Current:
		if cfg.Engine.Budget.ResetMode == "rolling" {
// Replace with:
		if cfg.Engine.Budget.ResetMode == budget.ResetModeRolling {
```

Also update the default value assignment (in the defaults block where `ResetMode` is set) if it uses the string literal `"calendar"` — replace with `budget.ResetModeCalendar`.

Add import `"github.com/dvflw/mantle/internal/budget"` to config.go.

- [ ] **Step 4: Run tests to verify no regression**

Run: `go test ./internal/budget/ ./internal/config/ -v`
Expected: PASS — identical behavior

- [ ] **Step 5: Commit**

```bash
git add internal/budget/budget.go internal/config/config.go
git commit -m "refactor(budget): extract ResetModeCalendar/ResetModeRolling constants"
```

---

### Task 6: Create `internal/cli/docker.go` — Docker operations

**Files:**
- Create: `internal/cli/docker.go`
- Create: `internal/cli/docker_test.go`

- [ ] **Step 1: Write failing tests**

In `internal/cli/docker_test.go`:

```go
package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDockerRunArgs(t *testing.T) {
	args := dockerRunArgs()
	assert.Equal(t, []string{
		"run", "-d",
		"--name", "mantle-postgres",
		"-p", "5432:5432",
		"-e", "POSTGRES_USER=mantle",
		"-e", "POSTGRES_PASSWORD=mantle",
		"-e", "POSTGRES_DB=mantle",
		"-v", "mantle-pgdata:/var/lib/postgresql/data",
		"postgres:16-alpine",
	}, args)
}

func TestParseHostFromURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{"standard", "postgres://mantle:mantle@localhost:5432/mantle", "localhost"},
		{"remote", "postgres://user:pass@db.example.com:5432/mydb", "db.example.com"},
		{"ipv4", "postgres://user:pass@10.0.0.1:5432/mydb", "10.0.0.1"},
		{"ipv6", "postgres://user:pass@[::1]:5432/mydb", "::1"},
		{"no-port", "postgres://user:pass@myhost/mydb", "myhost"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, parseHostFromURL(tt.url))
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cli/ -run "TestDockerRunArgs|TestParseHostFromURL" -v`
Expected: FAIL — functions not defined

- [ ] **Step 3: Write implementation**

In `internal/cli/docker.go`:

```go
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
	case "exited", "created", "dead":
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cli/ -run "TestDockerRunArgs|TestParseHostFromURL" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/cli/docker.go internal/cli/docker_test.go
git commit -m "feat(cli): add Docker auto-provisioning helpers for mantle init"
```

---

### Task 7: Implement connection recovery in `internal/cli/init.go`

**Files:**
- Modify: `internal/cli/init.go`
- Create: `internal/cli/init_test.go`

- [ ] **Step 1: Write tests for non-interactive mode and isInteractive**

In `internal/cli/init_test.go`:

```go
package cli

import (
	"bytes"
	"testing"

	"github.com/dvflw/mantle/internal/config"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestIsInteractive_ReturnsBool(t *testing.T) {
	// In test context, stdin is not a TTY — isInteractive should return false.
	assert.False(t, isInteractive())
}

func TestHandleConnectionFailure_NonInteractive_ReturnsError(t *testing.T) {
	// When stdin is not a TTY, handleConnectionFailure should return the
	// connection error immediately without prompting.
	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	cfg := &config.Config{}
	cfg.Database.URL = "postgres://mantle:mantle@localhost:5432/mantle"

	_, err := handleConnectionFailure(cmd, cfg, fmt.Errorf("connection refused"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
	// No prompt text should have been written to stdout.
	assert.Empty(t, buf.String())
}
```

Add `"fmt"` to imports.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cli/ -run "TestIsInteractive|TestHandleConnectionFailure_NonInteractive" -v`
Expected: FAIL — functions not defined yet

- [ ] **Step 3: Rewrite init.go with connection recovery flow**

Replace the contents of `internal/cli/init.go` with:

```go
package cli

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/dvflw/mantle/internal/netutil"
	"github.com/spf13/cobra"
)

func newInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize Mantle — run database migrations",
		Long:  "Runs all pending database migrations to set up or upgrade the Mantle schema.\nIf Postgres is not reachable, offers to start one automatically via Docker.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.FromContext(cmd.Context())
			if cfg == nil {
				return fmt.Errorf("config not loaded")
			}

			database, err := db.Open(cfg.Database)
			if err != nil {
				database, err = handleConnectionFailure(cmd, cfg, err)
				if err != nil {
					return err
				}
			}
			defer database.Close()

			fmt.Fprintln(cmd.OutOrStdout(), "Running migrations...")
			if err := db.Migrate(cmd.Context(), database); err != nil {
				return fmt.Errorf("migration failed: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Migrations complete.")
			return nil
		},
	}
}

// handleConnectionFailure is called when the initial db.Open fails.
// It classifies the host and offers interactive recovery options.
func handleConnectionFailure(cmd *cobra.Command, cfg *config.Config, connErr error) (*sql.DB, error) {
	host := parseHostFromURL(cfg.Database.URL)

	// Non-interactive mode (piped stdin, CI): just return the error.
	if !isInteractive() {
		return nil, fmt.Errorf("failed to connect to database: %w", connErr)
	}

	if netutil.IsLoopback(host) {
		return handleLoopbackFailure(cmd, cfg, connErr)
	}
	return handleRemoteFailure(cmd, cfg, host, connErr)
}

// isInteractive returns true if stdin is a terminal (not piped).
func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// handleLoopbackFailure offers Docker auto-provisioning for localhost connections.
func handleLoopbackFailure(cmd *cobra.Command, cfg *config.Config, connErr error) (*sql.DB, error) {
	out := cmd.OutOrStdout()
	in := cmd.InOrStdin()

	fmt.Fprintf(out, "No Postgres found on localhost: %v\n\n", connErr)
	fmt.Fprint(out, "Start a Postgres container with Docker? [Y/n]: ")

	var answer string
	fmt.Fscanln(in, &answer)
	answer = strings.TrimSpace(strings.ToLower(answer))

	if answer != "" && answer != "y" && answer != "yes" {
		return promptConnectionStringOrRetryDocker(cmd, cfg)
	}

	// User accepted Docker provisioning.
	return attemptDockerProvisioning(cmd, cfg)
}

// attemptDockerProvisioning checks Docker availability and starts the container.
func attemptDockerProvisioning(cmd *cobra.Command, cfg *config.Config) (*sql.DB, error) {
	out := cmd.OutOrStdout()

	if !dockerAvailable() {
		fmt.Fprintln(out, "\nDocker isn't installed or isn't running.")
		return promptConnectionStringOrRetryDocker(cmd, cfg)
	}

	fmt.Fprintln(out, "Starting Postgres container...")
	if err := dockerStartPostgres(cfg.Database); err != nil {
		return nil, fmt.Errorf("docker provisioning failed: %w", err)
	}

	fmt.Fprintln(out, "Postgres is ready.")
	return db.Open(cfg.Database)
}

// promptConnectionStringOrRetryDocker offers [R]etry or [C]onnection string.
func promptConnectionStringOrRetryDocker(cmd *cobra.Command, cfg *config.Config) (*sql.DB, error) {
	out := cmd.OutOrStdout()
	in := cmd.InOrStdin()

	for {
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "  [R] Retry (install or start Docker first)")
		fmt.Fprintln(out, "  [C] Enter a Postgres connection string")
		fmt.Fprint(out, "\nChoice [R/c]: ")

		var choice string
		fmt.Fscanln(in, &choice)
		choice = strings.TrimSpace(strings.ToLower(choice))

		switch choice {
		case "c":
			return promptConnectionString(cmd, cfg)
		default:
			// Retry Docker provisioning.
			return attemptDockerProvisioning(cmd, cfg)
		}
	}
}

// promptConnectionString asks the user for a connection URL and validates it.
func promptConnectionString(cmd *cobra.Command, cfg *config.Config) (*sql.DB, error) {
	out := cmd.OutOrStdout()
	in := cmd.InOrStdin()

	for {
		fmt.Fprint(out, "Postgres connection string: ")

		var connStr string
		fmt.Fscanln(in, &connStr)
		connStr = strings.TrimSpace(connStr)

		if connStr == "" {
			continue
		}

		cfg.Database.URL = connStr
		database, err := db.Open(cfg.Database)
		if err != nil {
			fmt.Fprintf(out, "Connection failed: %v\n", err)
			continue
		}
		return database, nil
	}
}

// handleRemoteFailure shows the error and offers retry/quit for non-loopback hosts.
func handleRemoteFailure(cmd *cobra.Command, cfg *config.Config, host string, connErr error) (*sql.DB, error) {
	out := cmd.OutOrStdout()
	in := cmd.InOrStdin()

	for {
		fmt.Fprintf(out, "Failed to connect to database at %s\n\n", host)
		fmt.Fprintf(out, "  Error: %v\n\n", connErr)
		fmt.Fprintln(out, "  [R] Retry (fix the issue and try again)")
		fmt.Fprintln(out, "  [Q] Quit")
		fmt.Fprint(out, "\nChoice [R/q]: ")

		var choice string
		fmt.Fscanln(in, &choice)
		choice = strings.TrimSpace(strings.ToLower(choice))

		if choice == "q" {
			return nil, fmt.Errorf("failed to connect to database at %s: %w", host, connErr)
		}

		// Retry: re-load config to pick up env var / config file changes.
		newCfg, err := config.Load(cmd)
		if err != nil {
			fmt.Fprintf(out, "Config reload error: %v\n", err)
			continue
		}
		cfg.Database = newCfg.Database

		database, err := db.Open(cfg.Database)
		if err != nil {
			connErr = err
			host = parseHostFromURL(cfg.Database.URL)
			continue
		}
		return database, nil
	}
}
```

- [ ] **Step 4: Fix compilation — verify build succeeds**

Run: `go build ./internal/cli/`
Expected: success

- [ ] **Step 5: Run all CLI tests including the new ones**

Run: `go test ./internal/cli/ -v -short`
Expected: PASS — `TestIsInteractive`, `TestHandleConnectionFailure_NonInteractive`, `TestDockerRunArgs`, `TestParseHostFromURL` all pass

- [ ] **Step 6: Commit**

```bash
git add internal/cli/init.go internal/cli/init_test.go
git commit -m "feat(cli): add connection recovery flow to mantle init (#7)"
```

---

### Task 8: Update landing page quickstart

**Files:**
- Modify: `site/src/components/GetStarted.astro`

- [ ] **Step 1: Update the steps array**

In `site/src/components/GetStarted.astro`, replace the steps array (lines 2-23):

```javascript
const steps = [
  {
    number: '1',
    title: 'Install',
    code: 'go install github.com/dvflw/mantle/cmd/mantle@latest',
  },
  {
    number: '2',
    title: 'Initialize',
    code: 'mantle init\n# Starts Postgres via Docker if needed, then runs migrations',
  },
  {
    number: '3',
    title: 'Apply your first workflow',
    code: 'mantle apply examples/hello-world.yaml\n# Applied hello-world version 1',
  },
  {
    number: '4',
    title: 'Run it',
    code: 'mantle run hello-world\n# Running hello-world (version 1)...\n# Execution a1b2c3d4: completed\n#   fetch: completed (1.0s)',
  },
];
```

Key changes: Step 2 title changes from "Start Postgres and initialize" to "Initialize". The `docker compose up -d` line is removed. A comment explains what `mantle init` does.

- [ ] **Step 2: Verify the site builds**

Run: `cd site && npm run build` (or whatever the build command is — check `site/package.json`)
Expected: success

- [ ] **Step 3: Commit**

```bash
git add site/src/components/GetStarted.astro
git commit -m "docs(site): simplify quickstart — mantle init handles DB setup (#7)"
```

---

### Task 9: Update getting-started docs

**Files:**
- Modify: `site/src/content/docs/getting-started/index.md`

- [ ] **Step 1: Update prerequisites section**

Replace the prerequisites section (lines 9-22) — Docker is no longer required:

```markdown
## Prerequisites

You need the following installed on your machine:

- **Go 1.25+** -- [install instructions](https://go.dev/doc/install)
- **Docker** (optional) -- [install instructions](https://docs.docker.com/get-docker/) -- used for automatic local Postgres provisioning

Verify your setup:

```bash
go version    # go1.25 or later
```
```

- [ ] **Step 2: Update the install/start section**

Replace the "Install and Start" section (lines 24-43) with two paths — `go install` (primary) and clone (development):

```markdown
## Install and Start (< 2 minutes)

Install the binary and initialize:

```bash
go install github.com/dvflw/mantle/cmd/mantle@latest
mantle init
```

`mantle init` connects to Postgres and runs migrations. If no database is reachable on localhost, it offers to start one automatically via Docker. For remote databases, set the URL before running init:

```bash
export MANTLE_DATABASE_URL="postgres://mantle:secret@db.example.com:5432/mantle?sslmode=require"
mantle init
```

You should see:

```
Running migrations...
Migrations complete.
```

**Development setup:** If you want to build from source, clone the repository and use `make build` instead of `go install`:

```bash
git clone https://github.com/dvflw/mantle.git && cd mantle
make build
./mantle init
```

See [Configuration](/docs/configuration) for all database options.
```

Remove the paragraph about `docker compose up -d` and `sslmode=prefer` (lines 35-43). The new text covers both install paths and explains the Docker auto-provisioning.

- [ ] **Step 3: Verify the site builds**

Run: `cd site && npm run build`
Expected: success

- [ ] **Step 4: Commit**

```bash
git add site/src/content/docs/getting-started/index.md
git commit -m "docs: update getting-started guide for new mantle init flow (#7)"
```

---

### Task 10: Manual smoke test

- [ ] **Step 1: Build the binary**

```bash
cd /Users/michael/Development/mantle
make build
```

- [ ] **Step 2: Test happy path (Docker running, Postgres available)**

```bash
docker compose up -d   # ensure Postgres is running
./mantle init
```

Expected: "Running migrations... Migrations complete."

- [ ] **Step 3: Test Docker auto-provisioning (no Postgres running)**

```bash
docker compose down
docker rm -f mantle-postgres 2>/dev/null
./mantle init
```

Expected: prompts "Start a Postgres container with Docker? [Y/n]". Accept with Enter/Y. Should start container, wait for readiness, run migrations.

- [ ] **Step 4: Test non-interactive mode**

```bash
docker compose down
echo "" | ./mantle init
```

Expected: returns error immediately, no prompts.

- [ ] **Step 5: Test remote failure with retry**

```bash
MANTLE_DATABASE_URL="postgres://user:pass@db.doesnotexist.com:5432/mantle" ./mantle init
```

Expected: shows connection error with host, offers Retry/Quit. Press Q to quit.

- [ ] **Step 6: Run full test suite**

```bash
make test
make lint
```

Expected: all tests pass, no lint errors.

- [ ] **Step 7: Clean up and final commit if needed**

```bash
docker rm -f mantle-postgres 2>/dev/null
docker compose up -d  # restore normal dev state
```
