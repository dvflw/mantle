# Workflow Apply Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `mantle apply` that stores validated workflow definitions as immutable versioned records in Postgres with SHA-256 change detection.

**Architecture:** `internal/workflow/store.go` provides Save/GetLatestVersion/GetLatestHash with direct SQL. CLI command opens DB, reads file, parses, saves. Testcontainers for integration tests.

**Tech Stack:** Go, `database/sql`, `crypto/sha256`, pgx, testcontainers

**Spec:** `docs/superpowers/specs/2026-03-18-workflow-apply-design.md`

**Linear issue:** [DVFLW-229](https://linear.app/dvflw/issue/DVFLW-229/workflow-apply-with-versioning-mantle-apply)

---

### Task 1: Workflow store — database operations

**Files:**
- Create: `internal/workflow/store.go`
- Test: `internal/workflow/store_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/workflow/store_test.go`:

```go
package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/dvflw/mantle/internal/db"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("mantle_test"),
		postgres.WithUsername("mantle"),
		postgres.WithPassword("mantle"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Skipf("Could not start Postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	})

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("Failed to get connection string: %v", err)
	}

	database, err := db.Open(connStr)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { database.Close() })

	// Run migrations
	if err := db.Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	return database
}

func TestSave_NewWorkflow(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	raw := []byte(`
name: test-workflow
steps:
  - name: step-one
    action: http/request
    params:
      method: GET
      url: https://example.com
`)
	result, err := ParseBytes(raw)
	if err != nil {
		t.Fatalf("ParseBytes() error = %v", err)
	}

	version, err := Save(ctx, database, result, raw)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if version != 1 {
		t.Errorf("version = %d, want 1", version)
	}
}

func TestSave_NoChanges(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	raw := []byte(`
name: test-workflow
steps:
  - name: step-one
    action: http/request
    params:
      method: GET
      url: https://example.com
`)
	result, err := ParseBytes(raw)
	if err != nil {
		t.Fatalf("ParseBytes() error = %v", err)
	}

	// First save
	_, err = Save(ctx, database, result, raw)
	if err != nil {
		t.Fatalf("Save() first error = %v", err)
	}

	// Second save — same content
	version, err := Save(ctx, database, result, raw)
	if err != nil {
		t.Fatalf("Save() second error = %v", err)
	}
	if version != 0 {
		t.Errorf("version = %d, want 0 (no changes)", version)
	}
}

func TestSave_NewVersion(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	raw1 := []byte(`
name: test-workflow
steps:
  - name: step-one
    action: http/request
    params:
      method: GET
      url: https://example.com
`)
	result1, err := ParseBytes(raw1)
	if err != nil {
		t.Fatalf("ParseBytes() error = %v", err)
	}

	v1, err := Save(ctx, database, result1, raw1)
	if err != nil {
		t.Fatalf("Save() v1 error = %v", err)
	}
	if v1 != 1 {
		t.Errorf("v1 = %d, want 1", v1)
	}

	// Modified content
	raw2 := []byte(`
name: test-workflow
steps:
  - name: step-one
    action: http/request
    params:
      method: POST
      url: https://example.com
`)
	result2, err := ParseBytes(raw2)
	if err != nil {
		t.Fatalf("ParseBytes() error = %v", err)
	}

	v2, err := Save(ctx, database, result2, raw2)
	if err != nil {
		t.Fatalf("Save() v2 error = %v", err)
	}
	if v2 != 2 {
		t.Errorf("v2 = %d, want 2", v2)
	}
}

func TestSave_CorrectHash(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	raw := []byte(`
name: test-workflow
steps:
  - name: step-one
    action: http/request
`)
	result, err := ParseBytes(raw)
	if err != nil {
		t.Fatalf("ParseBytes() error = %v", err)
	}

	_, err = Save(ctx, database, result, raw)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	hash, err := GetLatestHash(ctx, database, "test-workflow")
	if err != nil {
		t.Fatalf("GetLatestHash() error = %v", err)
	}

	expected := sha256.Sum256(raw)
	expectedHex := hex.EncodeToString(expected[:])
	if hash != expectedHex {
		t.Errorf("hash = %q, want %q", hash, expectedHex)
	}
}

func TestGetLatestVersion_NoVersions(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	v, err := GetLatestVersion(ctx, database, "nonexistent")
	if err != nil {
		t.Fatalf("GetLatestVersion() error = %v", err)
	}
	if v != 0 {
		t.Errorf("version = %d, want 0", v)
	}
}
```

Note: The tests use `ParseBytes(raw)` — a new function that parses from bytes instead of a filename. We need to add this to the parser.

- [ ] **Step 2: Add `ParseBytes` to the parser**

Add to `internal/workflow/parse.go`:

```go
// ParseBytes parses workflow YAML from raw bytes.
func ParseBytes(data []byte) (*ParseResult, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parsing workflow: %w", err)
	}

	var w Workflow
	if err := root.Decode(&w); err != nil {
		return nil, fmt.Errorf("decoding workflow: %w", err)
	}

	return &ParseResult{
		Workflow: &w,
		Root:     &root,
	}, nil
}
```

- [ ] **Step 3: Write store implementation**

Create `internal/workflow/store.go`:

```go
package workflow

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Save validates, hashes, and stores a workflow definition.
// Returns the new version number. Returns 0 if content is unchanged.
func Save(ctx context.Context, database *sql.DB, result *ParseResult, rawContent []byte) (int, error) {
	// Validate
	if errs := Validate(result); len(errs) > 0 {
		return 0, fmt.Errorf("validation failed: %s", errs[0].Error())
	}

	w := result.Workflow

	// Hash raw content
	hash := sha256.Sum256(rawContent)
	contentHash := hex.EncodeToString(hash[:])

	// Check for changes
	latestHash, err := GetLatestHash(ctx, database, w.Name)
	if err != nil {
		return 0, fmt.Errorf("checking latest hash: %w", err)
	}
	if contentHash == latestHash {
		return 0, nil // No changes
	}

	// Get next version
	latestVersion, err := GetLatestVersion(ctx, database, w.Name)
	if err != nil {
		return 0, fmt.Errorf("getting latest version: %w", err)
	}
	nextVersion := latestVersion + 1

	// Serialize workflow to JSON for JSONB storage
	content, err := json.Marshal(w)
	if err != nil {
		return 0, fmt.Errorf("marshaling workflow: %w", err)
	}

	// Insert
	_, err = database.ExecContext(ctx,
		`INSERT INTO workflow_definitions (name, version, content, content_hash)
		 VALUES ($1, $2, $3, $4)`,
		w.Name, nextVersion, content, contentHash,
	)
	if err != nil {
		return 0, fmt.Errorf("inserting workflow definition: %w", err)
	}

	return nextVersion, nil
}

// GetLatestVersion returns the latest version number for a workflow name.
// Returns 0 if no versions exist.
func GetLatestVersion(ctx context.Context, database *sql.DB, name string) (int, error) {
	var version int
	err := database.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(version), 0) FROM workflow_definitions WHERE name = $1",
		name,
	).Scan(&version)
	if err != nil {
		return 0, err
	}
	return version, nil
}

// GetLatestHash returns the content_hash of the latest version for a workflow name.
// Returns empty string if no versions exist.
func GetLatestHash(ctx context.Context, database *sql.DB, name string) (string, error) {
	var hash sql.NullString
	err := database.QueryRowContext(ctx,
		"SELECT content_hash FROM workflow_definitions WHERE name = $1 ORDER BY version DESC LIMIT 1",
		name,
	).Scan(&hash)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return hash.String, nil
}
```

- [ ] **Step 4: Run tests**

Run:
```bash
go test ./internal/workflow/ -v -timeout 120s -run TestSave
```

Expected: PASS — all store tests pass.

- [ ] **Step 5: Run all tests**

Run:
```bash
go test ./... -v -timeout 120s
```

Expected: All tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/workflow/store.go internal/workflow/store_test.go internal/workflow/parse.go
git commit -m "feat: add workflow store with versioned save and change detection"
```

---

### Task 2: `mantle apply` CLI command

**Files:**
- Create: `internal/cli/apply.go`
- Modify: `internal/cli/root.go` (register command)

- [ ] **Step 1: Create apply command**

Create `internal/cli/apply.go`:

```go
package cli

import (
	"fmt"
	"os"

	"github.com/dvflw/mantle/internal/config"
	"github.com/dvflw/mantle/internal/db"
	"github.com/dvflw/mantle/internal/workflow"
	"github.com/spf13/cobra"
)

func newApplyCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "apply <file>",
		Short: "Apply a workflow definition",
		Long:  "Validates and stores a workflow definition as a new immutable version in the database.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filename := args[0]

			cfg := config.FromContext(cmd.Context())
			if cfg == nil {
				return fmt.Errorf("config not loaded")
			}

			database, err := db.Open(cfg.Database.URL)
			if err != nil {
				return fmt.Errorf("failed to connect to database: %w", err)
			}
			defer database.Close()

			// Read raw bytes for hashing
			rawContent, err := os.ReadFile(filename)
			if err != nil {
				return fmt.Errorf("reading %s: %w", filename, err)
			}

			// Parse
			result, err := workflow.ParseBytes(rawContent)
			if err != nil {
				return fmt.Errorf("parsing %s: %w", filename, err)
			}

			// Save
			version, err := workflow.Save(cmd.Context(), database, result, rawContent)
			if err != nil {
				return err
			}

			if version == 0 {
				latestVersion, _ := workflow.GetLatestVersion(cmd.Context(), database, result.Workflow.Name)
				fmt.Fprintf(cmd.OutOrStdout(), "No changes — %s is already at version %d\n",
					result.Workflow.Name, latestVersion)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Applied %s version %d\n",
					result.Workflow.Name, version)
			}

			return nil
		},
	}
}
```

- [ ] **Step 2: Register apply command on root**

Modify `internal/cli/root.go` — add after existing command registrations:

```go
	cmd.AddCommand(newApplyCommand())
```

- [ ] **Step 3: Verify help output**

Run:
```bash
go run ./cmd/mantle --help
```

Expected: Shows `apply` in available commands.

Run:
```bash
go run ./cmd/mantle apply --help
```

Expected: Shows "Apply a workflow definition" with `<file>` argument.

- [ ] **Step 4: Run all tests**

Run:
```bash
go test ./... -v -timeout 120s
```

Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/apply.go internal/cli/root.go
git commit -m "feat: add mantle apply command for versioned workflow storage"
```

---

### Task 3: Final verification

- [ ] **Step 1: Run all tests**

Run:
```bash
go test ./... -v -timeout 120s
```

Expected: All tests pass.

- [ ] **Step 2: Run go vet**

Run:
```bash
go vet ./...
```

Expected: No warnings.

- [ ] **Step 3: Build and verify CLI**

Run:
```bash
make build
./mantle --help
./mantle apply --help
./mantle validate examples/fetch-and-summarize.yaml
./mantle version
make clean
```

Expected: All commands work. Apply shows in help. Validate still works.
