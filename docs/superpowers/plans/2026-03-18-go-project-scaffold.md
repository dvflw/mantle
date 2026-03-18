# Go Project Scaffold Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Initialize Go project with module, Cobra CLI, and working `mantle version` command.

**Architecture:** Thin `cmd/mantle/main.go` entrypoint delegates to `internal/cli/` for Cobra commands. Version info lives in `internal/version/` with ldflags-injected variables. Makefile wraps build with git metadata injection.

**Tech Stack:** Go, Cobra (`github.com/spf13/cobra`)

**Spec:** `docs/superpowers/specs/2026-03-18-go-project-scaffold-design.md`

**Linear issue:** [DVFLW-218](https://linear.app/dvflw/issue/DVFLW-218/go-project-scaffold-and-cli-framework)

---

### Task 1: Initialize Go module

**Files:**
- Create: `go.mod`

- [ ] **Step 1: Initialize Go module**

Run:
```bash
cd /Users/michael/Development/mantle
go mod init github.com/dvflw/mantle
```

Expected: `go.mod` created with module path `github.com/dvflw/mantle`.

- [ ] **Step 2: Verify go.mod**

Run:
```bash
cat go.mod
```

Expected: Shows `module github.com/dvflw/mantle` and a Go version line.

- [ ] **Step 3: Commit**

```bash
git add go.mod
git commit -m "chore: initialize Go module github.com/dvflw/mantle"
```

---

### Task 2: Version package with ldflags variables

**Files:**
- Create: `internal/version/version.go`
- Test: `internal/version/version_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/version/version_test.go`:

```go
package version

import (
	"testing"
)

func TestString_Defaults(t *testing.T) {
	got := String()
	want := "mantle dev (none, built unknown)"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestString_WithValues(t *testing.T) {
	// Temporarily set version vars
	origVersion, origCommit, origDate := Version, Commit, Date
	Version = "v0.1.0"
	Commit = "abc1234"
	Date = "2026-03-18T15:30:00Z"
	defer func() {
		Version, Commit, Date = origVersion, origCommit, origDate
	}()

	got := String()
	want := "mantle v0.1.0 (abc1234, built 2026-03-18T15:30:00Z)"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/version/ -v
```

Expected: FAIL — package does not exist yet.

- [ ] **Step 3: Write minimal implementation**

Create `internal/version/version.go`:

```go
package version

import "fmt"

// Set via ldflags at build time.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// String returns a formatted version string.
func String() string {
	return fmt.Sprintf("mantle %s (%s, built %s)", Version, Commit, Date)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/version/ -v
```

Expected: PASS — both tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/version/
git commit -m "feat: add version package with ldflags-injected variables"
```

---

### Task 3: Root Cobra command

**Files:**
- Create: `internal/cli/root.go`
- Test: `internal/cli/root_test.go`

- [ ] **Step 1: Install Cobra dependency**

Run:
```bash
go get github.com/spf13/cobra
```

- [ ] **Step 2: Write the failing test**

Create `internal/cli/root_test.go`:

```go
package cli

import (
	"bytes"
	"testing"
)

func TestRootCommand_NoArgs(t *testing.T) {
	cmd := NewRootCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := buf.String()
	if len(output) == 0 {
		t.Error("expected help output, got empty string")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run:
```bash
go test ./internal/cli/ -v
```

Expected: FAIL — package does not exist yet.

- [ ] **Step 4: Write minimal implementation**

Create `internal/cli/root.go`:

```go
package cli

import (
	"github.com/spf13/cobra"
)

// NewRootCommand creates the root mantle CLI command.
func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mantle",
		Short: "Headless AI workflow automation platform",
		Long:  "Mantle is a headless AI workflow automation platform — BYOK, IaC-first, enterprise-grade, open source.",
		SilenceUsage: true,
	}

	cmd.PersistentFlags().String("config", "", "config file path (default: mantle.yaml)")

	return cmd
}
```

- [ ] **Step 5: Run test to verify it passes**

Run:
```bash
go test ./internal/cli/ -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/root.go internal/cli/root_test.go go.mod go.sum
git commit -m "feat: add root Cobra command with --config flag"
```

---

### Task 4: Version subcommand

**Files:**
- Create: `internal/cli/version.go`
- Modify: `internal/cli/root.go` (add version subcommand)
- Test: `internal/cli/version_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/cli/version_test.go`:

```go
package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	cmd := NewRootCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"version"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := buf.String()
	if !strings.HasPrefix(output, "mantle ") {
		t.Errorf("expected output to start with 'mantle ', got %q", output)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/cli/ -v -run TestVersionCommand
```

Expected: FAIL — `version` subcommand not registered.

- [ ] **Step 3: Write version command**

Create `internal/cli/version.go`:

```go
package cli

import (
	"fmt"

	"github.com/dvflw/mantle/internal/version"
	"github.com/spf13/cobra"
)

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), version.String())
			return nil
		},
	}
}
```

- [ ] **Step 4: Register version command on root**

Modify `internal/cli/root.go` — add this line inside `NewRootCommand()` before the `return cmd` statement:

```go
	cmd.AddCommand(newVersionCommand())
```

- [ ] **Step 5: Run tests to verify they pass**

Run:
```bash
go test ./internal/cli/ -v
```

Expected: PASS — both root and version tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/version.go internal/cli/version_test.go internal/cli/root.go
git commit -m "feat: add mantle version subcommand"
```

---

### Task 5: Main entrypoint

**Files:**
- Create: `cmd/mantle/main.go`

- [ ] **Step 1: Create main.go**

Create `cmd/mantle/main.go`:

```go
package main

import (
	"os"

	"github.com/dvflw/mantle/internal/cli"
)

func main() {
	cmd := cli.NewRootCommand()
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Build and test manually**

Run:
```bash
go build -o mantle ./cmd/mantle && ./mantle version
```

Expected: `mantle dev (none, built unknown)`

- [ ] **Step 3: Test help output**

Run:
```bash
./mantle --help
```

Expected: Shows "Headless AI workflow automation platform" with `version` listed as available command and `--config` as a global flag.

- [ ] **Step 4: Commit**

```bash
git add cmd/mantle/main.go
git commit -m "feat: add CLI entrypoint"
```

---

### Task 6: Makefile

**Files:**
- Create: `Makefile`

- [ ] **Step 1: Create Makefile**

Create `Makefile`:

```makefile
VERSION ?= $(shell git describe --tags 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X github.com/dvflw/mantle/internal/version.Version=$(VERSION) \
           -X github.com/dvflw/mantle/internal/version.Commit=$(COMMIT) \
           -X github.com/dvflw/mantle/internal/version.Date=$(DATE)

.PHONY: build test lint clean

build:
	go build -ldflags "$(LDFLAGS)" -o mantle ./cmd/mantle

test:
	go test ./...

lint:
	golangci-lint run

clean:
	rm -f mantle
```

- [ ] **Step 2: Test `make build`**

Run:
```bash
make build && ./mantle version
```

Expected: Output shows current git commit hash and build date (e.g., `mantle dev (abc1234, built 2026-03-18T20:00:00Z)`).

- [ ] **Step 3: Test `make test`**

Run:
```bash
make test
```

Expected: All tests pass.

- [ ] **Step 4: Clean up binary**

Run:
```bash
make clean
```

Expected: `mantle` binary removed.

- [ ] **Step 5: Commit**

```bash
git add Makefile
git commit -m "chore: add Makefile with build, test, lint, clean targets"
```

---

### Task 7: Final verification

- [ ] **Step 1: Run all tests**

Run:
```bash
go test ./... -v
```

Expected: All tests in `internal/version/` and `internal/cli/` pass.

- [ ] **Step 2: Build with make and verify version**

Run:
```bash
make build && ./mantle version
```

Expected: `mantle dev (<commit-hash>, built <timestamp>)`

- [ ] **Step 3: Verify help output**

Run:
```bash
./mantle --help
```

Expected: Shows description, `version` subcommand, and `--config` flag.

- [ ] **Step 4: Verify subcommand help**

Run:
```bash
./mantle version --help
```

Expected: Shows "Print version information".

- [ ] **Step 5: Clean up**

Run:
```bash
make clean
```
