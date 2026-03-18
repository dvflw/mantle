# Workflow Validation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add workflow YAML parsing with line-number errors, structural validation, and `mantle validate` CLI command.

**Architecture:** `internal/workflow/` package owns structs, parsing (yaml.v3 with Node tree), and validation. CLI command is offline (no config/DB). Parser returns `ParseResult` with both the decoded struct and the yaml.Node tree for line-number lookups.

**Tech Stack:** Go, `gopkg.in/yaml.v3`

**Spec:** `docs/superpowers/specs/2026-03-18-workflow-validation-design.md`

**Linear issue:** [DVFLW-227](https://linear.app/dvflw/issue/DVFLW-227/offline-workflow-validation-mantle-validate)

---

### Task 1: Workflow structs and ValidationError

**Files:**
- Create: `internal/workflow/workflow.go`

- [ ] **Step 1: Create the workflow package with types**

Create `internal/workflow/workflow.go`:

```go
package workflow

import "fmt"

// Workflow represents a parsed workflow definition.
type Workflow struct {
	Name        string           `yaml:"name"`
	Description string           `yaml:"description"`
	Inputs      map[string]Input `yaml:"inputs"`
	Steps       []Step           `yaml:"steps"`
}

// Input represents a typed workflow input parameter.
type Input struct {
	Type        string `yaml:"type"`
	Description string `yaml:"description"`
}

// Step represents a single step in a workflow.
type Step struct {
	Name    string         `yaml:"name"`
	Action  string         `yaml:"action"`
	Params  map[string]any `yaml:"params"`
	If      string         `yaml:"if"`
	Retry   *RetryPolicy   `yaml:"retry"`
	Timeout string         `yaml:"timeout"`
}

// RetryPolicy configures retry behavior for a step.
type RetryPolicy struct {
	MaxAttempts int    `yaml:"max_attempts"`
	Backoff     string `yaml:"backoff"`
}

// ValidationError represents a validation error with optional location info.
type ValidationError struct {
	Line    int
	Column  int
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%d:%d: error: %s (%s)", e.Line, e.Column, e.Message, e.Field)
	}
	return fmt.Sprintf("error: %s (%s)", e.Message, e.Field)
}
```

- [ ] **Step 2: Verify it compiles**

Run:
```bash
go build ./internal/workflow/
```

Expected: Builds successfully.

- [ ] **Step 3: Commit**

```bash
git add internal/workflow/workflow.go
git commit -m "feat: add workflow structs and ValidationError type"
```

---

### Task 2: YAML parser with line numbers

**Files:**
- Create: `internal/workflow/parse.go`
- Test: `internal/workflow/parse_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/workflow/parse_test.go`:

```go
package workflow

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	return path
}

func TestParse_ValidWorkflow(t *testing.T) {
	path := writeTestFile(t, `
name: test-workflow
description: A test workflow

inputs:
  url:
    type: string
    description: The URL to fetch

steps:
  - name: fetch-data
    action: http/request
    params:
      method: GET
      url: "{{ inputs.url }}"
`)

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if result.Workflow.Name != "test-workflow" {
		t.Errorf("Name = %q, want test-workflow", result.Workflow.Name)
	}
	if result.Workflow.Description != "A test workflow" {
		t.Errorf("Description = %q, want 'A test workflow'", result.Workflow.Description)
	}
	if len(result.Workflow.Steps) != 1 {
		t.Fatalf("len(Steps) = %d, want 1", len(result.Workflow.Steps))
	}
	if result.Workflow.Steps[0].Name != "fetch-data" {
		t.Errorf("Steps[0].Name = %q, want fetch-data", result.Workflow.Steps[0].Name)
	}
	if result.Workflow.Steps[0].Action != "http/request" {
		t.Errorf("Steps[0].Action = %q, want http/request", result.Workflow.Steps[0].Action)
	}
	if result.Root == nil {
		t.Error("Root yaml.Node is nil")
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	path := writeTestFile(t, `
name: test
steps:
  - name: [invalid yaml
`)

	_, err := Parse(path)
	if err == nil {
		t.Fatal("Parse() expected error for invalid YAML, got nil")
	}
}

func TestParse_FileNotFound(t *testing.T) {
	_, err := Parse("/nonexistent/workflow.yaml")
	if err == nil {
		t.Fatal("Parse() expected error for missing file, got nil")
	}
}

func TestParse_WithRetryAndTimeout(t *testing.T) {
	path := writeTestFile(t, `
name: retry-workflow
steps:
  - name: flaky-step
    action: http/request
    params:
      method: GET
      url: https://example.com
    retry:
      max_attempts: 3
      backoff: exponential
    timeout: 30s
`)

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	step := result.Workflow.Steps[0]
	if step.Retry == nil {
		t.Fatal("Retry is nil")
	}
	if step.Retry.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", step.Retry.MaxAttempts)
	}
	if step.Retry.Backoff != "exponential" {
		t.Errorf("Backoff = %q, want exponential", step.Retry.Backoff)
	}
	if step.Timeout != "30s" {
		t.Errorf("Timeout = %q, want 30s", step.Timeout)
	}
}

func TestParse_WithConditional(t *testing.T) {
	path := writeTestFile(t, `
name: conditional-workflow
steps:
  - name: maybe-step
    action: http/request
    if: "steps.prev.output.status == 200"
    params:
      method: POST
      url: https://example.com
`)

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if result.Workflow.Steps[0].If != "steps.prev.output.status == 200" {
		t.Errorf("If = %q, want CEL expression", result.Workflow.Steps[0].If)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/workflow/ -v
```

Expected: FAIL — `Parse` function doesn't exist.

- [ ] **Step 3: Write implementation**

Create `internal/workflow/parse.go`:

```go
package workflow

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ParseResult holds a parsed workflow and the raw YAML node tree for line numbers.
type ParseResult struct {
	Workflow *Workflow
	Root     *yaml.Node
}

// Parse reads and parses a workflow YAML file.
// Returns the parsed workflow and the yaml.Node tree for line-number lookups.
func Parse(filename string) (*ParseResult, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", filename, err)
	}

	// First pass: parse into yaml.Node to preserve line numbers
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", filename, err)
	}

	// Second pass: decode into Workflow struct
	var w Workflow
	if err := root.Decode(&w); err != nil {
		return nil, fmt.Errorf("decoding %s: %w", filename, err)
	}

	return &ParseResult{
		Workflow: &w,
		Root:     &root,
	}, nil
}
```

- [ ] **Step 4: Add yaml.v3 as direct dependency**

Run:
```bash
go get gopkg.in/yaml.v3
go mod tidy
```

- [ ] **Step 5: Run tests to verify they pass**

Run:
```bash
go test ./internal/workflow/ -v
```

Expected: PASS — all parse tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/workflow/parse.go internal/workflow/parse_test.go go.mod go.sum
git commit -m "feat: add workflow YAML parser with line number preservation"
```

---

### Task 3: Structural validator

**Files:**
- Create: `internal/workflow/validate.go`
- Test: `internal/workflow/validate_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/workflow/validate_test.go`:

```go
package workflow

import (
	"testing"
)

func mustParse(t *testing.T, content string) *ParseResult {
	t.Helper()
	path := writeTestFile(t, content)
	result, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	return result
}

func TestValidate_ValidWorkflow(t *testing.T) {
	result := mustParse(t, `
name: test-workflow
steps:
  - name: step-one
    action: http/request
    params:
      method: GET
      url: https://example.com
`)

	errs := Validate(result)
	if len(errs) != 0 {
		t.Errorf("Validate() returned %d errors, want 0: %v", len(errs), errs)
	}
}

func TestValidate_MissingName(t *testing.T) {
	result := mustParse(t, `
steps:
  - name: step-one
    action: http/request
`)

	errs := Validate(result)
	if len(errs) == 0 {
		t.Fatal("Validate() expected errors for missing name")
	}
	assertHasError(t, errs, "name")
}

func TestValidate_InvalidNameFormat(t *testing.T) {
	result := mustParse(t, `
name: My Workflow!
steps:
  - name: step-one
    action: http/request
`)

	errs := Validate(result)
	assertHasError(t, errs, "name")
}

func TestValidate_MissingSteps(t *testing.T) {
	result := mustParse(t, `
name: test-workflow
`)

	errs := Validate(result)
	assertHasError(t, errs, "steps")
}

func TestValidate_EmptySteps(t *testing.T) {
	result := mustParse(t, `
name: test-workflow
steps: []
`)

	errs := Validate(result)
	assertHasError(t, errs, "steps")
}

func TestValidate_StepMissingName(t *testing.T) {
	result := mustParse(t, `
name: test-workflow
steps:
  - action: http/request
`)

	errs := Validate(result)
	assertHasError(t, errs, "steps[0].name")
}

func TestValidate_StepInvalidName(t *testing.T) {
	result := mustParse(t, `
name: test-workflow
steps:
  - name: Step One
    action: http/request
`)

	errs := Validate(result)
	assertHasError(t, errs, "steps[0].name")
}

func TestValidate_StepMissingAction(t *testing.T) {
	result := mustParse(t, `
name: test-workflow
steps:
  - name: step-one
`)

	errs := Validate(result)
	assertHasError(t, errs, "steps[0].action")
}

func TestValidate_DuplicateStepNames(t *testing.T) {
	result := mustParse(t, `
name: test-workflow
steps:
  - name: step-one
    action: http/request
  - name: step-one
    action: http/request
`)

	errs := Validate(result)
	assertHasError(t, errs, "steps[1].name")
}

func TestValidate_InvalidInputType(t *testing.T) {
	result := mustParse(t, `
name: test-workflow
inputs:
  count:
    type: integer
steps:
  - name: step-one
    action: http/request
`)

	errs := Validate(result)
	assertHasError(t, errs, "inputs.count.type")
}

func TestValidate_InvalidInputName(t *testing.T) {
	result := mustParse(t, `
name: test-workflow
inputs:
  my-input:
    type: string
steps:
  - name: step-one
    action: http/request
`)

	errs := Validate(result)
	assertHasError(t, errs, "inputs.my-input")
}

func TestValidate_InvalidRetryBackoff(t *testing.T) {
	result := mustParse(t, `
name: test-workflow
steps:
  - name: step-one
    action: http/request
    retry:
      max_attempts: 3
      backoff: linear
`)

	errs := Validate(result)
	assertHasError(t, errs, "steps[0].retry.backoff")
}

func TestValidate_InvalidRetryMaxAttempts(t *testing.T) {
	result := mustParse(t, `
name: test-workflow
steps:
  - name: step-one
    action: http/request
    retry:
      max_attempts: 0
      backoff: fixed
`)

	errs := Validate(result)
	assertHasError(t, errs, "steps[0].retry.max_attempts")
}

func TestValidate_InvalidTimeout(t *testing.T) {
	result := mustParse(t, `
name: test-workflow
steps:
  - name: step-one
    action: http/request
    timeout: not-a-duration
`)

	errs := Validate(result)
	assertHasError(t, errs, "steps[0].timeout")
}

func TestValidate_NegativeTimeout(t *testing.T) {
	result := mustParse(t, `
name: test-workflow
steps:
  - name: step-one
    action: http/request
    timeout: -5s
`)

	errs := Validate(result)
	assertHasError(t, errs, "steps[0].timeout")
}

func TestValidate_ValidInputTypes(t *testing.T) {
	result := mustParse(t, `
name: test-workflow
inputs:
  name:
    type: string
  count:
    type: number
  flag:
    type: boolean
steps:
  - name: step-one
    action: http/request
`)

	errs := Validate(result)
	if len(errs) != 0 {
		t.Errorf("Validate() returned %d errors, want 0: %v", len(errs), errs)
	}
}

func assertHasError(t *testing.T, errs []ValidationError, field string) {
	t.Helper()
	for _, e := range errs {
		if e.Field == field {
			return
		}
	}
	t.Errorf("expected validation error for field %q, got errors: %v", field, errs)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/workflow/ -v -run TestValidate
```

Expected: FAIL — `Validate` function doesn't exist.

- [ ] **Step 3: Write implementation**

Create `internal/workflow/validate.go`:

```go
package workflow

import (
	"fmt"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	namePattern  = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	inputPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	validTypes   = map[string]bool{"string": true, "number": true, "boolean": true}
	validBackoff = map[string]bool{"fixed": true, "exponential": true}
)

// Validate checks a parsed workflow for structural correctness.
// Returns a slice of validation errors (empty if valid).
func Validate(result *ParseResult) []ValidationError {
	var errs []ValidationError
	w := result.Workflow

	// Workflow name
	if w.Name == "" {
		errs = append(errs, validationErr(result.Root, "name", "workflow name is required"))
	} else if !namePattern.MatchString(w.Name) {
		errs = append(errs, validationErr(result.Root, "name",
			fmt.Sprintf("workflow name %q must match [a-z][a-z0-9-]*", w.Name)))
	}

	// Steps
	if len(w.Steps) == 0 {
		errs = append(errs, validationErr(result.Root, "steps", "at least one step is required"))
	}

	seen := map[string]bool{}
	for i, step := range w.Steps {
		field := fmt.Sprintf("steps[%d]", i)

		// Step name
		if step.Name == "" {
			errs = append(errs, validationErr(result.Root, field+".name", "step name is required"))
		} else if !namePattern.MatchString(step.Name) {
			errs = append(errs, validationErr(result.Root, field+".name",
				fmt.Sprintf("step name %q must match [a-z][a-z0-9-]*", step.Name)))
		} else if seen[step.Name] {
			errs = append(errs, validationErr(result.Root, field+".name",
				fmt.Sprintf("duplicate step name %q", step.Name)))
		}
		seen[step.Name] = true

		// Step action
		if step.Action == "" {
			errs = append(errs, validationErr(result.Root, field+".action", "step action is required"))
		}

		// Retry policy
		if step.Retry != nil {
			if step.Retry.MaxAttempts <= 0 {
				errs = append(errs, validationErr(result.Root, field+".retry.max_attempts",
					"max_attempts must be greater than 0"))
			}
			if step.Retry.Backoff != "" && !validBackoff[step.Retry.Backoff] {
				errs = append(errs, validationErr(result.Root, field+".retry.backoff",
					fmt.Sprintf("unknown backoff %q, must be fixed or exponential", step.Retry.Backoff)))
			}
		}

		// Timeout
		if step.Timeout != "" {
			d, err := time.ParseDuration(step.Timeout)
			if err != nil {
				errs = append(errs, validationErr(result.Root, field+".timeout",
					fmt.Sprintf("invalid duration %q", step.Timeout)))
			} else if d <= 0 {
				errs = append(errs, validationErr(result.Root, field+".timeout",
					"timeout must be positive"))
			}
		}
	}

	// Inputs
	for name, input := range w.Inputs {
		field := fmt.Sprintf("inputs.%s", name)

		if !inputPattern.MatchString(name) {
			errs = append(errs, validationErr(result.Root, field,
				fmt.Sprintf("input name %q must match [a-z][a-z0-9_]*", name)))
		}

		if input.Type != "" && !validTypes[input.Type] {
			errs = append(errs, validationErr(result.Root, field+".type",
				fmt.Sprintf("unknown input type %q, must be string, number, or boolean", input.Type)))
		}
	}

	return errs
}

// validationErr creates a ValidationError, attempting to find line info from the yaml.Node tree.
func validationErr(root *yaml.Node, field, message string) ValidationError {
	line, col := findFieldPosition(root, field)
	return ValidationError{
		Line:    line,
		Column:  col,
		Field:   field,
		Message: message,
	}
}

// findFieldPosition searches the yaml.Node tree for a field path and returns its line/column.
// Returns (0, 0) if the field cannot be found.
func findFieldPosition(root *yaml.Node, field string) (int, int) {
	if root == nil {
		return 0, 0
	}

	// The root node from yaml.Unmarshal is a DocumentNode containing a MappingNode
	node := root
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		node = node.Content[0]
	}

	// For simple top-level fields like "name", "steps"
	if node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content)-1; i += 2 {
			if node.Content[i].Value == field {
				return node.Content[i].Line, node.Content[i].Column
			}
		}
	}

	// For nested fields, return the root document position as fallback
	return node.Line, node.Column
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
go test ./internal/workflow/ -v
```

Expected: PASS — all parse and validate tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/workflow/validate.go internal/workflow/validate_test.go
git commit -m "feat: add workflow structural validator with name format checks"
```

---

### Task 4: `mantle validate` CLI command

**Files:**
- Create: `internal/cli/validate.go`
- Modify: `internal/cli/root.go` (register command)

- [ ] **Step 1: Create validate command**

Create `internal/cli/validate.go`:

```go
package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dvflw/mantle/internal/workflow"
	"github.com/spf13/cobra"
)

func newValidateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <file>",
		Short: "Validate a workflow YAML file",
		Long:  "Checks a workflow definition for schema conformance offline. No database or network connection required.",
		Args:  cobra.ExactArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil // Skip config loading — validate is fully offline
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			filename := args[0]

			result, err := workflow.Parse(filename)
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "%s: %v\n", filepath.Base(filename), err)
				os.Exit(1)
			}

			errs := workflow.Validate(result)
			if len(errs) > 0 {
				for _, e := range errs {
					if e.Line > 0 {
						fmt.Fprintf(cmd.ErrOrStderr(), "%s:%d:%d: error: %s (%s)\n",
							filepath.Base(filename), e.Line, e.Column, e.Message, e.Field)
					} else {
						fmt.Fprintf(cmd.ErrOrStderr(), "%s: error: %s (%s)\n",
							filepath.Base(filename), e.Message, e.Field)
					}
				}
				os.Exit(1)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "%s: valid\n", filepath.Base(filename))
			return nil
		},
	}
}
```

- [ ] **Step 2: Register validate command on root**

Modify `internal/cli/root.go` — add after existing command registrations:

```go
	cmd.AddCommand(newValidateCommand())
```

- [ ] **Step 3: Verify help output**

Run:
```bash
go run ./cmd/mantle --help
```

Expected: Shows `validate` in available commands.

Run:
```bash
go run ./cmd/mantle validate --help
```

Expected: Shows "Validate a workflow YAML file" with `<file>` argument.

- [ ] **Step 4: Test with a valid workflow file**

Create a test file `/tmp/test-workflow.yaml`:
```yaml
name: test-workflow
description: A simple test

steps:
  - name: fetch-data
    action: http/request
    params:
      method: GET
      url: https://example.com
```

Run:
```bash
go run ./cmd/mantle validate /tmp/test-workflow.yaml
```

Expected: `test-workflow.yaml: valid`

- [ ] **Step 5: Test with an invalid workflow file**

Create a test file `/tmp/bad-workflow.yaml`:
```yaml
name: Bad Name!
steps: []
```

Run:
```bash
go run ./cmd/mantle validate /tmp/bad-workflow.yaml
```

Expected: Prints errors and exits with code 1.

- [ ] **Step 6: Run all tests**

Run:
```bash
go test ./... -v -timeout 120s
```

Expected: All tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/cli/validate.go internal/cli/root.go
git commit -m "feat: add mantle validate command for offline workflow validation"
```

---

### Task 5: Example workflow fixture

**Files:**
- Create: `examples/fetch-and-summarize.yaml`

- [ ] **Step 1: Create example workflow**

Create `examples/fetch-and-summarize.yaml`:

```yaml
name: fetch-and-summarize
description: Fetch data from an API and summarize it with an LLM

inputs:
  url:
    type: string
    description: URL to fetch

steps:
  - name: fetch-data
    action: http/request
    params:
      method: GET
      url: "{{ inputs.url }}"

  - name: summarize
    action: ai/completion
    params:
      provider: openai
      model: gpt-4o
      prompt: "Summarize this data: {{ steps.fetch-data.output.body }}"
      output_schema:
        type: object
        properties:
          summary:
            type: string
          key_points:
            type: array
            items:
              type: string

  - name: post-result
    action: http/request
    if: "steps.summarize.output.key_points.size() > 0"
    params:
      method: POST
      url: https://hooks.example.com/results
      body:
        summary: "{{ steps.summarize.output.summary }}"
        points: "{{ steps.summarize.output.key_points }}"
```

- [ ] **Step 2: Validate the example**

Run:
```bash
go run ./cmd/mantle validate examples/fetch-and-summarize.yaml
```

Expected: `fetch-and-summarize.yaml: valid`

- [ ] **Step 3: Commit**

```bash
git add examples/
git commit -m "feat: add example workflow fixture"
```

---

### Task 6: Final verification

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
./mantle validate --help
./mantle validate examples/fetch-and-summarize.yaml
./mantle version
make clean
```

Expected: All commands work correctly. Validate shows "valid" for example workflow.
