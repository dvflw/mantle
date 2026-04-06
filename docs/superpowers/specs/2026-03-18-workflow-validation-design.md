# Design: Offline Workflow Validation — `mantle validate`

> Linear issue: [DVFLW-227](https://linear.app/dvflw/issue/DVFLW-227/offline-workflow-validation-mantle-validate)
> Date: 2026-03-18

## Goal

Add workflow YAML parsing with line-number error reporting and structural validation. Add `mantle validate` CLI command for offline schema conformance checking.

## Acceptance Criteria

- `mantle validate workflow.yaml` checks schema conformance offline
- Reports errors with line numbers and descriptive messages
- Exits with non-zero code on validation failure
- No network calls or engine connection required

## Package Structure

```
internal/
  workflow/
    workflow.go        # Workflow, Step, Input, RetryPolicy structs
    parse.go           # Parse(filename) — YAML parsing with yaml.Node line tracking
    validate.go        # Validate(*ParseResult) — structural validation rules
    parse_test.go      # Parsing tests
    validate_test.go   # Validation tests
  cli/
    validate.go        # mantle validate command
```

## Workflow Structs — `internal/workflow/workflow.go`

```go
type Workflow struct {
    Name        string            `yaml:"name"`
    Description string            `yaml:"description"`
    Inputs      map[string]Input  `yaml:"inputs"`
    Steps       []Step            `yaml:"steps"`
}

type Input struct {
    Type        string `yaml:"type"`
    Description string `yaml:"description"`
}

type Step struct {
    Name    string         `yaml:"name"`
    Action  string         `yaml:"action"`
    Params  map[string]any `yaml:"params"`
    If      string         `yaml:"if"`
    Retry   *RetryPolicy   `yaml:"retry"`
    Timeout string         `yaml:"timeout"`
}

type RetryPolicy struct {
    MaxAttempts int    `yaml:"max_attempts"`
    Backoff     string `yaml:"backoff"`
}
```

## ValidationError

```go
type ValidationError struct {
    Line    int    // 0 if unknown
    Column  int    // 0 if unknown
    Field   string // e.g., "steps[0].name"
    Message string
}

func (e ValidationError) Error() string {
    // Format: "line:col: error: message (field)"
}
```

## Parser — `internal/workflow/parse.go`

```go
type ParseResult struct {
    Workflow *Workflow
    Root     *yaml.Node // preserved for line number lookups
}

func Parse(filename string) (*ParseResult, error)
```

- Reads file contents
- Unmarshals using `gopkg.in/yaml.v3` with a two-pass approach:
  1. `yaml.Unmarshal` into `yaml.Node` tree to capture line/column positions
  2. Decode the node tree into the `Workflow` struct
- Returns parse errors with line numbers if YAML is malformed
- Preserves the `yaml.Node` tree in `ParseResult` for the validator to reference line numbers

## Validator — `internal/workflow/validate.go`

```go
func Validate(result *ParseResult) []ValidationError
```

Structural validation rules:
- `name` is required, non-empty, and matches `^[a-z][a-z0-9-]*$` (lowercase alphanumeric with hyphens, used as CLI args and DB keys)
- `steps` is required and has at least one entry
- Each step has a non-empty `name` matching `^[a-z][a-z0-9-]*$` (same format as workflow name)
- Each step has a non-empty `action`
- Step names are unique within the workflow
- Input names must be valid identifiers matching `^[a-z][a-z0-9_]*$` (used in CEL as `inputs.<name>`)
- Input `type` is one of: `string`, `number`, `boolean` (if inputs are declared)
- Retry `backoff` is one of: `fixed`, `exponential` (if retry is set)
- Retry `max_attempts` is > 0 (if retry is set)
- `timeout` parses as a valid positive Go duration (if set)

The validator walks the `yaml.Node` tree to find line/column positions for each error.

## CLI Command — `internal/cli/validate.go`

`mantle validate <file>` — offline, no DB, no network.

- Has its own no-op `PersistentPreRunE` (like the version command) since it needs no config or DB
- Reads file path from args
- Calls `workflow.Parse()` then `workflow.Validate()`
- Prints errors to stderr in the format: `filename:line:col: error: message (field)`
- Exits 0 on success, 1 on validation failure

Output format on success:
```
workflow.yaml: valid
```

Output format on failure:
```
workflow.yaml:3:1: error: step name is required (steps[0].name)
workflow.yaml:7:5: error: unknown input type "integer", must be string, number, or boolean (inputs.url.type)
```

## Dependencies

- `gopkg.in/yaml.v3` — YAML parsing with line number support (already a transitive dep via viper, but add as direct)

## What's NOT Included

- CEL expression syntax validation (needs expression engine)
- Action/connector-specific parameter validation (needs connector registry)
- JSON Schema document generation (can add later for editor support)
- Trigger declaration validation (Phase 5)
