# AI Tool Use & Parallel Execution (Phase 8) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable LLM steps to call tools during execution with deterministic crash recovery, and enable independent workflow steps to run in parallel via DAG resolution.

**Architecture:** DAG-based step scheduling replaces sequential execution. The orchestrator resolves `depends_on` declarations (explicit + implicit from CEL) to determine ready steps. AI tool use leverages Phase 7's sub-step rows (`parent_step_id`) — the AI connector acts as a mini-orchestrator for tool calls, caching LLM responses for crash recovery.

**Tech Stack:** Go, Postgres (sub-step rows, JSONB caching), OpenAI function calling API, testcontainers.

**Spec:** `docs/superpowers/specs/2026-03-19-ai-tool-use-parallel-execution-design.md`
**Depends on:** Phase 7 (Multi-Node Distribution) — `docs/superpowers/plans/2026-03-19-multi-node-distribution.md`

---

## File Structure

### New Files
- `internal/engine/dag.go` — DAG building, topological sort, ready-step resolution
- `internal/engine/dag_test.go` — DAG tests (cycles, implicit deps, conditional steps, failure cascading)
- `internal/connector/tools.go` — tool-use loop: LLM↔tool orchestration, sub-step creation, recovery
- `internal/connector/tools_test.go` — tool-use loop tests with mock LLM

### Modified Files
- `internal/workflow/workflow.go` — add Tool struct, tool-related fields
- `internal/workflow/validate.go` — add cycle detection, implicit dependency extraction, tool validation
- `internal/workflow/validate_test.go` — validation tests for deps and tools
- `internal/connector/ai.go` — extend for function calling, delegate to tools.go for multi-turn
- `internal/connector/ai_test.go` — tests for tool-use AI completion
- `internal/engine/orchestrator.go` — integrate DAG resolution into step scheduling
- `internal/engine/orchestrator_test.go` — tests for DAG-based orchestration
- `internal/metrics/metrics.go` — add tool-use metrics
- `internal/cli/logs.go` — render sub-steps in tree format

---

## Task 1: DAG Data Structures and Resolution

**Files:**
- Create: `internal/engine/dag.go`
- Create: `internal/engine/dag_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/engine/dag_test.go`:

```go
package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/dvflw/mantle/internal/workflow"
)

func TestDAG_LinearChain(t *testing.T) {
	steps := []workflow.Step{
		{Name: "a"},
		{Name: "b", DependsOn: []string{"a"}},
		{Name: "c", DependsOn: []string{"b"}},
	}
	dag, err := BuildDAG(steps)
	require.NoError(t, err)

	ready := dag.ReadySteps(map[string]string{})
	assert.Equal(t, []string{"a"}, ready)

	ready = dag.ReadySteps(map[string]string{"a": "completed"})
	assert.Equal(t, []string{"b"}, ready)

	ready = dag.ReadySteps(map[string]string{"a": "completed", "b": "completed"})
	assert.Equal(t, []string{"c"}, ready)
}

func TestDAG_ParallelSteps(t *testing.T) {
	steps := []workflow.Step{
		{Name: "a"},
		{Name: "b"},
		{Name: "c", DependsOn: []string{"a", "b"}},
	}
	dag, err := BuildDAG(steps)
	require.NoError(t, err)

	ready := dag.ReadySteps(map[string]string{})
	assert.ElementsMatch(t, []string{"a", "b"}, ready)

	ready = dag.ReadySteps(map[string]string{"a": "completed"})
	assert.Empty(t, ready) // b not done yet, c depends on both

	ready = dag.ReadySteps(map[string]string{"a": "completed", "b": "completed"})
	assert.Equal(t, []string{"c"}, ready)
}

func TestDAG_SkippedDependency(t *testing.T) {
	steps := []workflow.Step{
		{Name: "a"},
		{Name: "b", DependsOn: []string{"a"}},
	}
	dag, err := BuildDAG(steps)
	require.NoError(t, err)

	// Skipped counts as resolved
	ready := dag.ReadySteps(map[string]string{"a": "skipped"})
	assert.Equal(t, []string{"b"}, ready)
}

func TestDAG_CycleDetection(t *testing.T) {
	steps := []workflow.Step{
		{Name: "a", DependsOn: []string{"c"}},
		{Name: "b", DependsOn: []string{"a"}},
		{Name: "c", DependsOn: []string{"b"}},
	}
	_, err := BuildDAG(steps)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestDAG_FailureCascade(t *testing.T) {
	steps := []workflow.Step{
		{Name: "a"},
		{Name: "b"},
		{Name: "c", DependsOn: []string{"a"}},
		{Name: "d", DependsOn: []string{"b"}},
		{Name: "e", DependsOn: []string{"c", "d"}},
	}
	dag, err := BuildDAG(steps)
	require.NoError(t, err)

	// a failed — c and e should be cancelled, but d (depends on b only) is fine
	cancelled := dag.CascadeCancellations(map[string]string{"a": "failed", "b": "completed"})
	assert.ElementsMatch(t, []string{"c", "e"}, cancelled)
}

func TestDAG_UndefinedDependency(t *testing.T) {
	steps := []workflow.Step{
		{Name: "a", DependsOn: []string{"nonexistent"}},
	}
	_, err := BuildDAG(steps)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/engine/ -run TestDAG -v`
Expected: FAIL — `BuildDAG` doesn't exist.

- [ ] **Step 3: Write the DAG implementation**

Create `internal/engine/dag.go`:

```go
package engine

import (
	"fmt"
	"sort"

	"github.com/dvflw/mantle/internal/workflow"
)

// DAG represents a directed acyclic graph of workflow steps.
type DAG struct {
	steps    map[string]*workflow.Step
	deps     map[string][]string // step -> dependencies
	rdeps    map[string][]string // step -> reverse dependencies (dependents)
	order    []string            // topological order
}

// BuildDAG constructs a DAG from workflow steps.
// Returns an error if cycles are detected or dependencies reference undefined steps.
func BuildDAG(steps []workflow.Step) (*DAG, error) {
	d := &DAG{
		steps: make(map[string]*workflow.Step),
		deps:  make(map[string][]string),
		rdeps: make(map[string][]string),
	}

	for i := range steps {
		d.steps[steps[i].Name] = &steps[i]
		d.deps[steps[i].Name] = steps[i].DependsOn
	}

	// Validate dependencies reference existing steps
	for name, depList := range d.deps {
		for _, dep := range depList {
			if _, ok := d.steps[dep]; !ok {
				return nil, fmt.Errorf("step %q depends on undefined step %q", name, dep)
			}
			d.rdeps[dep] = append(d.rdeps[dep], name)
		}
	}

	// Topological sort with cycle detection (Kahn's algorithm)
	inDegree := make(map[string]int)
	for name := range d.steps {
		inDegree[name] = len(d.deps[name])
	}

	var queue []string
	for name, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, name)
		}
	}
	sort.Strings(queue) // deterministic ordering

	var order []string
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		order = append(order, node)

		for _, dependent := range d.rdeps[node] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
				sort.Strings(queue)
			}
		}
	}

	if len(order) != len(d.steps) {
		return nil, fmt.Errorf("dependency cycle detected in workflow steps")
	}

	d.order = order
	return d, nil
}

// ReadySteps returns step names that have all dependencies resolved.
// A dependency is resolved if its status is "completed" or "skipped".
// "cancelled" and "failed" are NOT resolved — their dependents are handled
// by CascadeCancellations instead.
func (d *DAG) ReadySteps(statuses map[string]string) []string {
	resolved := func(status string) bool {
		return status == "completed" || status == "skipped"
	}

	var ready []string
	for _, name := range d.order {
		if _, exists := statuses[name]; exists {
			continue // already has a status — skip
		}
		allDepsResolved := true
		for _, dep := range d.deps[name] {
			if !resolved(statuses[dep]) {
				allDepsResolved = false
				break
			}
		}
		if allDepsResolved {
			ready = append(ready, name)
		}
	}
	return ready
}

// CascadeCancellations returns step names that should be cancelled because
// a dependency (transitively) has failed.
func (d *DAG) CascadeCancellations(statuses map[string]string) []string {
	// Build set of failed steps + their transitive dependents
	poisoned := make(map[string]bool)
	for name, status := range statuses {
		if status == "failed" {
			poisoned[name] = true
		}
	}

	// Walk forward through topological order
	var cancelled []string
	for _, name := range d.order {
		if poisoned[name] {
			continue // already marked
		}
		if _, exists := statuses[name]; exists {
			continue // already has a terminal status
		}
		for _, dep := range d.deps[name] {
			if poisoned[dep] {
				poisoned[name] = true
				cancelled = append(cancelled, name)
				break
			}
		}
	}
	return cancelled
}

// AddImplicitDeps merges implicit dependencies (from CEL expression analysis)
// into the DAG. Call this after BuildDAG.
func (d *DAG) AddImplicitDeps(implicit map[string][]string) error {
	for step, deps := range implicit {
		if _, ok := d.steps[step]; !ok {
			continue
		}
		for _, dep := range deps {
			if _, ok := d.steps[dep]; !ok {
				return fmt.Errorf("implicit dependency: step %q references undefined step %q", step, dep)
			}
			// Deduplicate
			found := false
			for _, existing := range d.deps[step] {
				if existing == dep {
					found = true
					break
				}
			}
			if !found {
				d.deps[step] = append(d.deps[step], dep)
				d.rdeps[dep] = append(d.rdeps[dep], step)
			}
		}
	}

	// Re-validate: check for cycles after adding implicit deps
	// (rebuild topological sort)
	inDegree := make(map[string]int)
	for name := range d.steps {
		inDegree[name] = len(d.deps[name])
	}

	var queue []string
	for name, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, name)
		}
	}
	sort.Strings(queue)

	var order []string
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		order = append(order, node)
		for _, dependent := range d.rdeps[node] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
				sort.Strings(queue)
			}
		}
	}

	if len(order) != len(d.steps) {
		return fmt.Errorf("dependency cycle detected after adding implicit dependencies")
	}
	d.order = order
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/engine/ -run TestDAG -v`
Expected: All DAG tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/engine/dag.go internal/engine/dag_test.go
git commit -m "feat(phase8): add DAG resolution for parallel step execution"
```

---

## Task 2: Implicit Dependency Extraction from CEL

**Files:**
- Modify: `internal/workflow/validate.go`
- Modify: `internal/workflow/validate_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/workflow/validate_test.go`:

```go
func TestExtractImplicitDeps(t *testing.T) {
	tests := []struct {
		name     string
		params   map[string]any
		ifExpr   string
		expected []string
	}{
		{
			name:     "param references step output",
			params:   map[string]any{"url": "{{ steps.fetch_data.output.url }}"},
			expected: []string{"fetch_data"},
		},
		{
			name:     "if condition references step",
			params:   map[string]any{},
			ifExpr:   "steps.check.output.ready == true",
			expected: []string{"check"},
		},
		{
			name:     "nested param references",
			params:   map[string]any{"body": map[string]any{"data": "{{ steps.a.output.x }} and {{ steps.b.output.y }}"}},
			expected: []string{"a", "b"},
		},
		{
			name:     "no references",
			params:   map[string]any{"url": "https://example.com"},
			expected: nil,
		},
		{
			name:     "deduplicates",
			params:   map[string]any{"a": "{{ steps.x.output.a }}", "b": "{{ steps.x.output.b }}"},
			expected: []string{"x"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := Step{Params: tt.params, If: tt.ifExpr}
			deps := ExtractImplicitDeps(step)
			assert.ElementsMatch(t, tt.expected, deps)
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/workflow/ -run TestExtractImplicitDeps -v`
Expected: FAIL — `ExtractImplicitDeps` doesn't exist.

- [ ] **Step 3: Write the implementation**

Add to `internal/workflow/validate.go`:

```go
import "regexp"

var stepRefPattern = regexp.MustCompile(`steps\.(\w+)`)

// ExtractImplicitDeps extracts step names referenced in CEL expressions
// within a step's params and if condition. Only static references are detected.
func ExtractImplicitDeps(step Step) []string {
	seen := make(map[string]bool)
	var refs []string

	// Scan if condition
	if step.If != "" {
		for _, match := range stepRefPattern.FindAllStringSubmatch(step.If, -1) {
			if !seen[match[1]] {
				seen[match[1]] = true
				refs = append(refs, match[1])
			}
		}
	}

	// Scan params recursively
	scanParamsForRefs(step.Params, seen, &refs)

	return refs
}

func scanParamsForRefs(params map[string]any, seen map[string]bool, refs *[]string) {
	for _, v := range params {
		switch val := v.(type) {
		case string:
			for _, match := range stepRefPattern.FindAllStringSubmatch(val, -1) {
				if !seen[match[1]] {
					seen[match[1]] = true
					*refs = append(*refs, match[1])
				}
			}
		case map[string]any:
			scanParamsForRefs(val, seen, refs)
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/workflow/ -run TestExtractImplicitDeps -v`
Expected: PASS

- [ ] **Step 5: Integrate into validation**

In the `Validate` function in `internal/workflow/validate.go`, after existing checks, add cycle detection with implicit deps:

```go
// Build dependency graph including implicit deps
allDeps := make(map[string][]string)
for _, step := range wf.Steps {
	explicit := step.DependsOn
	implicit := ExtractImplicitDeps(step)
	// Merge and deduplicate
	merged := mergeUnique(explicit, implicit)
	allDeps[step.Name] = merged
}

// Check for undefined step references
stepNames := make(map[string]bool)
for _, step := range wf.Steps {
	stepNames[step.Name] = true
}
for stepName, deps := range allDeps {
	for _, dep := range deps {
		if !stepNames[dep] {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("steps.%s.depends_on", stepName),
				Message: fmt.Sprintf("depends on undefined step %q", dep),
			})
		}
	}
}
```

- [ ] **Step 6: Run full validation test suite**

Run: `go test ./internal/workflow/ -v`
Expected: All tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/workflow/validate.go internal/workflow/validate_test.go
git commit -m "feat(phase8): extract implicit dependencies from CEL expressions"
```

---

## Task 3: Tool Struct and YAML Parsing

**Files:**
- Modify: `internal/workflow/workflow.go`
- Modify: `internal/workflow/parse.go`
- Modify: `internal/workflow/parse_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/workflow/parse_test.go`:

```go
func TestParse_ToolUseStep(t *testing.T) {
	yaml := `
name: tool-workflow
steps:
  - name: agent
    action: ai/completion
    params:
      model: gpt-4o
      prompt: "Find weather"
      max_tool_rounds: 5
      max_tool_calls_per_round: 3
      tools:
        - name: get_weather
          description: "Get weather for a city"
          input_schema:
            type: object
            properties:
              city:
                type: string
            required:
              - city
          action: http/request
          params:
            url: "https://api.weather.com/{{ tool_input.city }}"
`
	wf, err := Parse([]byte(yaml))
	require.NoError(t, err)
	require.Len(t, wf.Steps, 1)

	step := wf.Steps[0]
	tools, ok := step.Params["tools"]
	require.True(t, ok)

	toolList, ok := tools.([]any)
	require.True(t, ok)
	require.Len(t, toolList, 1)
}
```

- [ ] **Step 2: Run test to verify it passes (YAML parsing is generic)**

Run: `go test ./internal/workflow/ -run TestParse_ToolUseStep -v`
Expected: This should already pass since params are `map[string]any` and YAML parsing handles nested structures. If it fails, adjust the test or parsing.

- [ ] **Step 3: Add Tool struct for typed access**

In `internal/workflow/workflow.go`, add:

```go
// Tool represents a tool that an AI step can invoke.
type Tool struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	InputSchema map[string]any `yaml:"input_schema"`
	Action      string         `yaml:"action"`
	Params      map[string]any `yaml:"params"`
}

// ParseTools extracts typed Tool structs from an AI step's params.
func ParseTools(params map[string]any) ([]Tool, error) {
	toolsRaw, ok := params["tools"]
	if !ok {
		return nil, nil
	}

	toolList, ok := toolsRaw.([]any)
	if !ok {
		return nil, fmt.Errorf("tools must be an array")
	}

	var tools []Tool
	for _, item := range toolList {
		toolMap, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("each tool must be a map")
		}

		tool := Tool{}
		if v, ok := toolMap["name"].(string); ok {
			tool.Name = v
		}
		if v, ok := toolMap["description"].(string); ok {
			tool.Description = v
		}
		if v, ok := toolMap["input_schema"].(map[string]any); ok {
			tool.InputSchema = v
		}
		if v, ok := toolMap["action"].(string); ok {
			tool.Action = v
		}
		if v, ok := toolMap["params"].(map[string]any); ok {
			tool.Params = v
		}

		tools = append(tools, tool)
	}
	return tools, nil
}
```

- [ ] **Step 4: Add a typed test for ParseTools**

```go
func TestParseTools(t *testing.T) {
	params := map[string]any{
		"tools": []any{
			map[string]any{
				"name":        "get_weather",
				"description": "Get weather",
				"input_schema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"city": map[string]any{"type": "string"},
					},
				},
				"action": "http/request",
				"params": map[string]any{"url": "https://example.com"},
			},
		},
	}

	tools, err := ParseTools(params)
	require.NoError(t, err)
	require.Len(t, tools, 1)
	assert.Equal(t, "get_weather", tools[0].Name)
	assert.Equal(t, "Get weather", tools[0].Description)
	assert.Equal(t, "http/request", tools[0].Action)
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/workflow/ -v`
Expected: All pass.

- [ ] **Step 6: Commit**

```bash
git add internal/workflow/workflow.go internal/workflow/parse.go internal/workflow/parse_test.go
git commit -m "feat(phase8): add Tool struct and YAML parsing for AI tool use"
```

---

## Task 4: Tool Validation

**Files:**
- Modify: `internal/workflow/validate.go`
- Modify: `internal/workflow/validate_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/workflow/validate_test.go`:

```go
func TestValidate_ToolUseDuplicateNames(t *testing.T) {
	result := mustParse(t, `
name: dup-tools
steps:
  - name: agent
    action: ai/completion
    params:
      model: gpt-4o
      prompt: test
      tools:
        - name: fetch
          description: "fetch 1"
          input_schema: {type: object}
          action: http/request
          params: {url: "https://a.com"}
        - name: fetch
          description: "fetch 2"
          input_schema: {type: object}
          action: http/request
          params: {url: "https://b.com"}
`)
	errs := Validate(result)
	assert.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Message, "duplicate tool name")
}

func TestValidate_ToolMissingDescription(t *testing.T) {
	result := mustParse(t, `
name: no-desc
steps:
  - name: agent
    action: ai/completion
    params:
      model: gpt-4o
      prompt: test
      tools:
        - name: fetch
          input_schema: {type: object}
          action: http/request
          params: {url: "https://a.com"}
`)
	errs := Validate(result)
	assert.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Message, "description")
}

func TestValidate_ToolRoundsOutOfBounds(t *testing.T) {
	result := mustParse(t, `
name: too-many-rounds
steps:
  - name: agent
    action: ai/completion
    params:
      model: gpt-4o
      prompt: test
      max_tool_rounds: 100
`)
	errs := Validate(result)
	assert.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Message, "max_tool_rounds")
}

Note: `mustParse` and `Validate` use the existing `ParseResult` pattern. `mustParse(t, content)` writes YAML to a temp file and calls `Parse(path)`. `Validate` takes `*ParseResult`, not `*Workflow`.
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/workflow/ -run "TestValidate_Tool" -v`
Expected: FAIL — validation not implemented yet.

- [ ] **Step 3: Add tool validation to Validate function**

In `internal/workflow/validate.go`, add a tool validation block within the step validation loop:

```go
// Validate tools on AI steps
if step.Action == "ai/completion" {
	tools, err := ParseTools(step.Params)
	if err != nil {
		errors = append(errors, ValidationError{
			Field:   fmt.Sprintf("steps.%s.params.tools", step.Name),
			Message: err.Error(),
		})
	} else if len(tools) > 0 {
		toolNames := make(map[string]bool)
		for _, tool := range tools {
			if tool.Name == "" {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("steps.%s.params.tools", step.Name),
					Message: "tool name is required",
				})
			}
			if toolNames[tool.Name] {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("steps.%s.params.tools.%s", step.Name, tool.Name),
					Message: fmt.Sprintf("duplicate tool name %q", tool.Name),
				})
			}
			toolNames[tool.Name] = true
			if tool.Description == "" {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("steps.%s.params.tools.%s", step.Name, tool.Name),
					Message: fmt.Sprintf("tool %q requires a description for LLM function calling", tool.Name),
				})
			}
			if tool.InputSchema == nil {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("steps.%s.params.tools.%s", step.Name, tool.Name),
					Message: fmt.Sprintf("tool %q requires input_schema", tool.Name),
				})
			}
			if tool.Action == "" {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("steps.%s.params.tools.%s", step.Name, tool.Name),
					Message: fmt.Sprintf("tool %q requires an action", tool.Name),
				})
			}
		}
	}

	// Validate max_tool_rounds
	if v, ok := step.Params["max_tool_rounds"]; ok {
		if rounds, ok := v.(int); ok && rounds > 50 {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("steps.%s.params.max_tool_rounds", step.Name),
				Message: "max_tool_rounds must not exceed 50",
			})
		}
	}
	if v, ok := step.Params["max_tool_calls_per_round"]; ok {
		if calls, ok := v.(int); ok && calls > 25 {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("steps.%s.params.max_tool_calls_per_round", step.Name),
				Message: "max_tool_calls_per_round must not exceed 25",
			})
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/workflow/ -run "TestValidate_Tool" -v`
Expected: All pass.

- [ ] **Step 5: Run full validation test suite**

Run: `go test ./internal/workflow/ -v`
Expected: All pass.

- [ ] **Step 6: Commit**

```bash
git add internal/workflow/validate.go internal/workflow/validate_test.go
git commit -m "feat(phase8): add tool validation for AI steps"
```

---

## Task 5: AI Connector Tool Use — Function Calling

**Files:**
- Modify: `internal/connector/ai.go`
- Modify: `internal/connector/ai_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/connector/ai_test.go`:

```go
func TestAIConnector_FunctionCallingRequest(t *testing.T) {
	// Mock server that returns a tool call response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)

		// Verify tools were sent
		tools, ok := req["tools"]
		require.True(t, ok, "tools should be in request")
		toolList := tools.([]any)
		assert.Len(t, toolList, 1)

		// Return a tool call response
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"content": nil,
					"tool_calls": []map[string]any{{
						"id":   "call_123",
						"type": "function",
						"function": map[string]any{
							"name":      "get_weather",
							"arguments": `{"city":"London"}`,
						},
					}},
				},
			}},
			"model": "gpt-4o",
			"usage": map[string]any{
				"prompt_tokens":     100,
				"completion_tokens": 20,
				"total_tokens":      120,
			},
		})
	}))
	defer server.Close()

	conn := &AIConnector{Client: server.Client()}
	result, err := conn.Execute(context.Background(), map[string]any{
		"model":    "gpt-4o",
		"prompt":   "What's the weather?",
		"base_url": server.URL,
		"_credential": map[string]string{"api_key": "test-key"},
		"_tools": []map[string]any{{
			"type": "function",
			"function": map[string]any{
				"name":        "get_weather",
				"description": "Get weather",
				"parameters":  map[string]any{"type": "object", "properties": map[string]any{"city": map[string]any{"type": "string"}}},
			},
		}},
	})
	require.NoError(t, err)

	// Should return tool calls in output
	toolCalls, ok := result["tool_calls"]
	require.True(t, ok)
	calls := toolCalls.([]any)
	assert.Len(t, calls, 1)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/connector/ -run TestAIConnector_FunctionCalling -v`
Expected: FAIL — `_tools` param not handled.

- [ ] **Step 3: Extend AI connector to send tools and parse tool call responses**

In `internal/connector/ai.go`, extend the `Execute` method:

1. Check for `_tools` in params — if present, add `"tools"` field to the request body.
2. Parse the response for `tool_calls` in the message — if present, return them in the output instead of text.

Add to the request body building:
```go
if tools, ok := params["_tools"]; ok {
	body["tools"] = tools
}
```

Extend response parsing to handle tool calls:
```go
if len(choice.Message.ToolCalls) > 0 {
	output["tool_calls"] = choice.Message.ToolCalls
	output["finish_reason"] = "tool_calls"
} else {
	output["text"] = choice.Message.Content
	output["finish_reason"] = "stop"
}
```

Update the response structs:
```go
type chatMessage struct {
	Content   string     `json:"content"`
	ToolCalls []toolCall `json:"tool_calls"`
}

type toolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function toolFunction `json:"function"`
}

type toolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/connector/ -run TestAIConnector -v`
Expected: All AI connector tests pass (both old and new).

- [ ] **Step 5: Commit**

```bash
git add internal/connector/ai.go internal/connector/ai_test.go
git commit -m "feat(phase8): extend AI connector with function calling support"
```

---

## Task 6: Tool Use Loop Orchestration

**Files:**
- Create: `internal/connector/tools.go`
- Create: `internal/connector/tools_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/connector/tools_test.go`:

```go
package connector

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolLoop_SingleRound(t *testing.T) {
	// Mock AI connector that returns tool call, then final response
	callCount := 0
	mockAI := func(ctx context.Context, params map[string]any) (map[string]any, error) {
		callCount++
		if callCount == 1 {
			return map[string]any{
				"tool_calls": []any{
					map[string]any{
						"id": "call_1", "type": "function",
						"function": map[string]any{"name": "get_weather", "arguments": `{"city":"London"}`},
					},
				},
				"finish_reason": "tool_calls",
			}, nil
		}
		return map[string]any{
			"text":          "The weather in London is sunny.",
			"finish_reason": "stop",
		}, nil
	}

	// Mock tool executor
	mockToolExecutor := func(ctx context.Context, toolName string, args map[string]any) (map[string]any, error) {
		assert.Equal(t, "get_weather", toolName)
		assert.Equal(t, "London", args["city"])
		return map[string]any{"temp": "22C", "condition": "sunny"}, nil
	}

	loop := &ToolLoop{
		AIExecute:    mockAI,
		ToolExecutor: mockToolExecutor,
		MaxRounds:    10,
		MaxCallsPerRound: 10,
	}

	result, err := loop.Run(context.Background(), map[string]any{
		"model":  "gpt-4o",
		"prompt": "What's the weather?",
	})
	require.NoError(t, err)
	assert.Equal(t, "The weather in London is sunny.", result["text"])
	assert.Equal(t, 2, callCount) // 1 tool call + 1 final
}

func TestToolLoop_MaxRoundsEnforced(t *testing.T) {
	// Mock AI that always returns tool calls
	mockAI := func(ctx context.Context, params map[string]any) (map[string]any, error) {
		return map[string]any{
			"tool_calls": []any{
				map[string]any{
					"id": "call_1", "type": "function",
					"function": map[string]any{"name": "fetch", "arguments": `{}`},
				},
			},
			"finish_reason": "tool_calls",
		}, nil
	}

	mockToolExecutor := func(ctx context.Context, toolName string, args map[string]any) (map[string]any, error) {
		return map[string]any{"data": "result"}, nil
	}

	loop := &ToolLoop{
		AIExecute:    mockAI,
		ToolExecutor: mockToolExecutor,
		MaxRounds:    2,
		MaxCallsPerRound: 10,
	}

	_, err := loop.Run(context.Background(), map[string]any{
		"model":  "gpt-4o",
		"prompt": "infinite loop",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tool use limit")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/connector/ -run TestToolLoop -v`
Expected: FAIL — `ToolLoop` doesn't exist.

- [ ] **Step 3: Write the ToolLoop implementation**

Create `internal/connector/tools.go`:

```go
package connector

import (
	"context"
	"encoding/json"
	"fmt"
)

// ToolExecutor executes a tool by name with the given arguments.
type ToolExecutor func(ctx context.Context, toolName string, args map[string]any) (map[string]any, error)

// AIExecuteFunc calls the AI/LLM with params and returns the response.
type AIExecuteFunc func(ctx context.Context, params map[string]any) (map[string]any, error)

// ToolLoop orchestrates the LLM↔tool interaction loop.
type ToolLoop struct {
	AIExecute        AIExecuteFunc
	ToolExecutor     ToolExecutor
	MaxRounds        int
	MaxCallsPerRound int
	// OnLLMResponse is called after each LLM response for caching (crash recovery).
	OnLLMResponse func(response map[string]any) error
	// OnToolResult is called after each tool execution for tracking.
	OnToolResult func(toolName string, round int, result map[string]any) error
}

// Run executes the tool-use loop until the LLM returns a final response
// or limits are exceeded.
func (tl *ToolLoop) Run(ctx context.Context, initialParams map[string]any) (map[string]any, error) {
	messages := buildInitialMessages(initialParams)
	params := copyParams(initialParams)

	for round := 0; round < tl.MaxRounds; round++ {
		params["_messages"] = messages

		result, err := tl.AIExecute(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("LLM call (round %d): %w", round, err)
		}

		// Cache LLM response
		if tl.OnLLMResponse != nil {
			if err := tl.OnLLMResponse(result); err != nil {
				return nil, fmt.Errorf("cache LLM response: %w", err)
			}
		}

		// Check for tool calls
		toolCalls, hasTools := result["tool_calls"]
		if !hasTools || result["finish_reason"] != "tool_calls" {
			return result, nil // Final response
		}

		calls, ok := toolCalls.([]any)
		if !ok || len(calls) == 0 {
			return result, nil
		}

		if len(calls) > tl.MaxCallsPerRound {
			return nil, fmt.Errorf("LLM returned %d tool calls, exceeding max_tool_calls_per_round (%d)", len(calls), tl.MaxCallsPerRound)
		}

		// Append assistant message with tool calls
		messages = append(messages, map[string]any{
			"role":       "assistant",
			"tool_calls": calls,
		})

		// Execute each tool call
		for _, callRaw := range calls {
			call, ok := callRaw.(map[string]any)
			if !ok {
				continue
			}

			funcInfo, _ := call["function"].(map[string]any)
			toolName, _ := funcInfo["name"].(string)
			argsStr, _ := funcInfo["arguments"].(string)
			callID, _ := call["id"].(string)

			var args map[string]any
			if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
				// Return error to LLM as tool result
				messages = append(messages, map[string]any{
					"role":         "tool",
					"tool_call_id": callID,
					"content":      fmt.Sprintf("Invalid arguments: %v", err),
				})
				continue
			}

			toolResult, toolErr := tl.ToolExecutor(ctx, toolName, args)
			if toolErr != nil {
				messages = append(messages, map[string]any{
					"role":         "tool",
					"tool_call_id": callID,
					"content":      fmt.Sprintf("Tool error: %v", toolErr),
				})
				continue
			}

			if tl.OnToolResult != nil {
				tl.OnToolResult(toolName, round, toolResult)
			}

			resultJSON, _ := json.Marshal(toolResult)
			messages = append(messages, map[string]any{
				"role":         "tool",
				"tool_call_id": callID,
				"content":      string(resultJSON),
			})
		}
	}

	// Graceful limit: ask LLM for best response with available information
	params["_messages"] = append(messages, map[string]any{
		"role":    "user",
		"content": "Tool use limit reached. Provide your best response with the information available.",
	})
	finalResult, err := tl.AIExecute(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("final LLM call after limit: %w", err)
	}
	// If LLM still returns tool calls, fail
	if _, hasTools := finalResult["tool_calls"]; hasTools {
		return nil, fmt.Errorf("tool use limit reached after %d rounds and LLM still requesting tools", tl.MaxRounds)
	}
	return finalResult, nil
}

func buildInitialMessages(params map[string]any) []map[string]any {
	var messages []map[string]any
	if sp, ok := params["system_prompt"].(string); ok && sp != "" {
		messages = append(messages, map[string]any{"role": "system", "content": sp})
	}
	if p, ok := params["prompt"].(string); ok {
		messages = append(messages, map[string]any{"role": "user", "content": p})
	}
	return messages
}

func copyParams(params map[string]any) map[string]any {
	cp := make(map[string]any)
	for k, v := range params {
		cp[k] = v
	}
	return cp
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/connector/ -run TestToolLoop -v`
Expected: All pass.

- [ ] **Step 5: Commit**

```bash
git add internal/connector/tools.go internal/connector/tools_test.go
git commit -m "feat(phase8): add tool-use loop orchestration for AI steps"
```

---

## Task 7: Sub-Step DB Persistence and Crash Recovery

**Files:**
- Create: `internal/engine/toolsteps.go`
- Create: `internal/engine/toolsteps_test.go`

This task implements the critical DB persistence layer for tool-use sub-steps and the crash recovery algorithm from the design spec. The `ToolLoop` (Task 6) uses callbacks; this task provides the DB-backed implementations.

- [ ] **Step 1: Write the failing tests**

Create `internal/engine/toolsteps_test.go`:

```go
package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolSteps_CreateSubStep(t *testing.T) {
	db := setupTestDB(t)
	ts := &ToolSteps{DB: db}

	execID := createTestExecution(t, db)
	insertPendingStep(t, db, execID, "agent", 1)

	// Get the parent step ID
	var parentID string
	db.QueryRow(`SELECT id FROM step_executions WHERE execution_id = $1 AND step_name = 'agent'`, execID).Scan(&parentID)

	err := ts.CreateSubStep(context.Background(), execID, parentID, "agent/tool/get_weather/0", 1)
	require.NoError(t, err)

	var count int
	db.QueryRow(`SELECT COUNT(*) FROM step_executions WHERE parent_step_id = $1`, parentID).Scan(&count)
	assert.Equal(t, 1, count)
}

func TestToolSteps_CreateSubStep_Idempotent(t *testing.T) {
	db := setupTestDB(t)
	ts := &ToolSteps{DB: db}

	execID := createTestExecution(t, db)
	insertPendingStep(t, db, execID, "agent", 1)

	var parentID string
	db.QueryRow(`SELECT id FROM step_executions WHERE execution_id = $1 AND step_name = 'agent'`, execID).Scan(&parentID)

	// Create twice — should not error
	ts.CreateSubStep(context.Background(), execID, parentID, "agent/tool/get_weather/0", 1)
	err := ts.CreateSubStep(context.Background(), execID, parentID, "agent/tool/get_weather/0", 1)
	require.NoError(t, err)

	var count int
	db.QueryRow(`SELECT COUNT(*) FROM step_executions WHERE parent_step_id = $1`, parentID).Scan(&count)
	assert.Equal(t, 1, count) // Only one row
}

func TestToolSteps_CacheLLMResponse(t *testing.T) {
	db := setupTestDB(t)
	ts := &ToolSteps{DB: db}

	execID := createTestExecution(t, db)
	insertPendingStep(t, db, execID, "agent", 1)

	var stepID string
	db.QueryRow(`SELECT id FROM step_executions WHERE execution_id = $1 AND step_name = 'agent'`, execID).Scan(&stepID)

	resp := map[string]any{"tool_calls": []any{map[string]any{"name": "fetch"}}}
	err := ts.CacheLLMResponse(context.Background(), stepID, resp)
	require.NoError(t, err)

	// Verify cached
	cached, err := ts.LoadCachedLLMResponses(context.Background(), stepID)
	require.NoError(t, err)
	assert.Len(t, cached, 1)
}

func TestToolSteps_LoadSubStepStatuses(t *testing.T) {
	db := setupTestDB(t)
	ts := &ToolSteps{DB: db}

	execID := createTestExecution(t, db)
	insertPendingStep(t, db, execID, "agent", 1)

	var parentID string
	db.QueryRow(`SELECT id FROM step_executions WHERE execution_id = $1 AND step_name = 'agent'`, execID).Scan(&parentID)

	ts.CreateSubStep(context.Background(), execID, parentID, "agent/tool/get_weather/0", 1)

	statuses, err := ts.LoadSubStepStatuses(context.Background(), parentID)
	require.NoError(t, err)
	assert.Equal(t, "pending", statuses["agent/tool/get_weather/0"].Status)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/engine/ -run TestToolSteps -v`
Expected: FAIL — `ToolSteps` type doesn't exist.

- [ ] **Step 3: Write the ToolSteps implementation**

Create `internal/engine/toolsteps.go`:

```go
package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// ToolSteps handles DB persistence for AI tool-use sub-steps and crash recovery.
type ToolSteps struct {
	DB *sql.DB
}

// CreateSubStep inserts a child step_execution row for a tool call.
// Uses ON CONFLICT DO NOTHING for idempotency on crash recovery.
func (ts *ToolSteps) CreateSubStep(ctx context.Context, executionID, parentStepID, stepName string, maxAttempts int) error {
	_, err := ts.DB.ExecContext(ctx, `
		INSERT INTO step_executions (execution_id, parent_step_id, step_name, attempt, status, max_attempts)
		VALUES ($1, $2, $3, 1, 'pending', $4)
		ON CONFLICT (execution_id, step_name, attempt) DO NOTHING
	`, executionID, parentStepID, stepName, maxAttempts)
	return err
}

// CacheLLMResponse appends an LLM response to the cached_llm_responses JSONB array.
func (ts *ToolSteps) CacheLLMResponse(ctx context.Context, stepID string, response map[string]any) error {
	responseJSON, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("marshal LLM response: %w", err)
	}
	_, err = ts.DB.ExecContext(ctx, `
		UPDATE step_executions
		SET cached_llm_responses = cached_llm_responses || $2::jsonb, updated_at = NOW()
		WHERE id = $1
	`, stepID, responseJSON)
	return err
}

// LoadCachedLLMResponses loads all cached LLM responses for a step.
func (ts *ToolSteps) LoadCachedLLMResponses(ctx context.Context, stepID string) ([]map[string]any, error) {
	var raw string
	err := ts.DB.QueryRowContext(ctx, `
		SELECT cached_llm_responses FROM step_executions WHERE id = $1
	`, stepID).Scan(&raw)
	if err != nil {
		return nil, err
	}

	var responses []map[string]any
	if err := json.Unmarshal([]byte(raw), &responses); err != nil {
		return nil, fmt.Errorf("unmarshal cached responses: %w", err)
	}
	return responses, nil
}

// LoadSubStepStatuses loads all child step statuses for a parent step.
func (ts *ToolSteps) LoadSubStepStatuses(ctx context.Context, parentStepID string) (map[string]*StepStatus, error) {
	rows, err := ts.DB.QueryContext(ctx, `
		SELECT step_name, status, attempt, max_attempts, output, error
		FROM step_executions
		WHERE parent_step_id = $1
		ORDER BY created_at
	`, parentStepID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	statuses := make(map[string]*StepStatus)
	for rows.Next() {
		var name, status string
		var attempt, maxAttempts int
		var outputJSON, errMsg sql.NullString

		if err := rows.Scan(&name, &status, &attempt, &maxAttempts, &outputJSON, &errMsg); err != nil {
			return nil, err
		}

		ss := &StepStatus{Status: status, Attempt: attempt, MaxAttempts: maxAttempts}
		if outputJSON.Valid {
			json.Unmarshal([]byte(outputJSON.String), &ss.Output)
		}
		if errMsg.Valid {
			ss.Error = errMsg.String
		}
		statuses[name] = ss
	}
	return statuses, rows.Err()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/engine/ -run TestToolSteps -v`
Expected: All pass.

- [ ] **Step 5: Commit**

```bash
git add internal/engine/toolsteps.go internal/engine/toolsteps_test.go
git commit -m "feat(phase8): add sub-step DB persistence and LLM response caching"
```

---

## Task 8: Tool Input CEL Resolution

**Files:**
- Modify: `internal/cel/cel.go`
- Modify: `internal/cel/cel_test.go`

The `tool_input` CEL variable allows tool params to reference LLM-provided arguments (e.g., `{{ tool_input.city }}`).

- [ ] **Step 1: Write the failing test**

Add to `internal/cel/cel_test.go`:

```go
func TestEvaluator_ToolInput(t *testing.T) {
	eval := NewEvaluator()
	ctx := &Context{
		Steps:  map[string]map[string]any{},
		Inputs: map[string]any{},
	}

	toolInput := map[string]any{"city": "London", "units": "celsius"}
	result, err := eval.ResolveString(ctx, "https://api.weather.com/v1?city={{ tool_input.city }}&units={{ tool_input.units }}", toolInput)
	require.NoError(t, err)
	assert.Equal(t, "https://api.weather.com/v1?city=London&units=celsius", result)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cel/ -run TestEvaluator_ToolInput -v`
Expected: FAIL — `tool_input` not recognized in CEL context.

- [ ] **Step 3: Add tool_input support**

In `internal/cel/cel.go`, extend `ResolveString` (and `ResolveParams`) to accept an optional `toolInput map[string]any` parameter. When provided, register `tool_input` as a CEL variable alongside `steps`, `inputs`, and `env`. This makes `{{ tool_input.city }}` resolvable.

The change is small — add `tool_input` to the CEL environment declaration and the variable bindings when it's non-nil.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cel/ -v`
Expected: All tests pass (existing tests unaffected, new test passes).

- [ ] **Step 5: Commit**

```bash
git add internal/cel/cel.go internal/cel/cel_test.go
git commit -m "feat(phase8): add tool_input CEL variable for tool parameter resolution"
```

---

## Task 9: Engine Configuration for Tool Use

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/config/config_test.go`:

```go
func TestConfig_ToolUseDefaults(t *testing.T) {
	cmd := newTestCommand()
	cfg, err := Load(cmd)
	require.NoError(t, err)

	assert.Equal(t, 10, cfg.Engine.DefaultMaxToolRounds)
	assert.Equal(t, 10, cfg.Engine.DefaultMaxToolCallsPerRound)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestConfig_ToolUseDefaults -v`
Expected: FAIL — fields don't exist.

- [ ] **Step 3: Add fields to EngineConfig**

In `internal/config/config.go`, add to the EngineConfig struct (added in Phase 7):

```go
DefaultMaxToolRounds        int `mapstructure:"default_max_tool_rounds"`
DefaultMaxToolCallsPerRound int `mapstructure:"default_max_tool_calls_per_round"`
```

Add defaults and env var bindings:

```go
viper.SetDefault("engine.default_max_tool_rounds", 10)
viper.SetDefault("engine.default_max_tool_calls_per_round", 10)
viper.BindEnv("engine.default_max_tool_rounds", "MANTLE_ENGINE_DEFAULT_MAX_TOOL_ROUNDS")
viper.BindEnv("engine.default_max_tool_calls_per_round", "MANTLE_ENGINE_DEFAULT_MAX_TOOL_CALLS_PER_ROUND")
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/config/ -v`
Expected: All pass.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(phase8): add tool use configuration defaults"
```

---

## Task 10: Integrate DAG into Orchestrator

**Prerequisite:** Phase 7 must be implemented first. This task uses `Orchestrator`, `BuildDAG`, and related types from Phase 7's `internal/engine/orchestrator.go` and the DAG code from Task 1.

**Files:**
- Modify: `internal/engine/orchestrator.go` (created in Phase 7)
- Modify: `internal/engine/orchestrator_test.go` (created in Phase 7)

- [ ] **Step 1: Write the failing test**

Add to `internal/engine/orchestrator_test.go`:

```go
func TestOrchestrator_DAGBasedStepCreation(t *testing.T) {
	db := setupTestDB(t)
	o := &Orchestrator{DB: db, NodeID: "node-1", LeaseDuration: 120 * time.Second}

	execID := createTestExecution(t, db)

	steps := []workflow.Step{
		{Name: "a"},
		{Name: "b"},
		{Name: "c", DependsOn: []string{"a", "b"}},
	}

	dag, err := BuildDAG(steps)
	require.NoError(t, err)

	// Initially, a and b should be ready
	ready := dag.ReadySteps(map[string]string{})
	err = o.CreatePendingSteps(context.Background(), execID, ready, map[string]int{})
	require.NoError(t, err)

	var count int
	db.QueryRow(`SELECT COUNT(*) FROM step_executions WHERE execution_id = $1 AND status = 'pending'`, execID).Scan(&count)
	assert.Equal(t, 2, count)

	// After a and b complete, c should be ready
	// Simulate a and b completed
	db.Exec(`UPDATE step_executions SET status = 'completed' WHERE execution_id = $1`, execID)

	statuses, _ := o.GetStepStatuses(context.Background(), execID)
	statusMap := make(map[string]string)
	for name, ss := range statuses {
		statusMap[name] = ss.Status
	}

	ready2 := dag.ReadySteps(statusMap)
	assert.Equal(t, []string{"c"}, ready2)
}
```

- [ ] **Step 2: Run test to verify it passes**

Run: `go test ./internal/engine/ -run TestOrchestrator_DAG -v`
Expected: PASS — this test uses existing Orchestrator methods with the new DAG.

- [ ] **Step 3: Commit**

```bash
git add internal/engine/orchestrator_test.go
git commit -m "test(phase8): verify DAG integration with orchestrator"
```

---

## Task 11: Tool Use Metrics

**Files:**
- Modify: `internal/metrics/metrics.go`
- Modify: `internal/metrics/metrics_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/metrics/metrics_test.go`:

```go
func TestToolUseMetrics(t *testing.T) {
	RecordToolCall("agent", "get_weather", "completed")
	RecordToolRound("agent")
	RecordToolRoundDuration("agent", 500*time.Millisecond)
	RecordLLMCacheHit()
	SetParallelStepsInFlight(3)
	// Verify no panic
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/metrics/ -run TestToolUseMetrics -v`
Expected: FAIL — functions don't exist.

- [ ] **Step 3: Add tool-use metrics**

In `internal/metrics/metrics.go`, add:

```go
var (
	ToolCallsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "mantle_tool_calls_total",
		Help: "Total tool calls by step, tool, and status",
	}, []string{"step", "tool", "status"})

	ToolRoundsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "mantle_tool_rounds_total",
		Help: "Total tool use rounds by step",
	}, []string{"step"})

	ToolRoundDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "mantle_tool_round_duration_seconds",
		Help:    "Duration of tool use rounds",
		Buckets: prometheus.DefBuckets,
	}, []string{"step"})

	LLMCacheHitsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "mantle_llm_cache_hits_total",
		Help: "Total LLM response cache hits during recovery",
	})

	ParallelStepsInFlight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "mantle_parallel_steps_in_flight",
		Help: "Number of concurrent step executions per workflow",
	})
)

func RecordToolCall(step, tool, status string) {
	ToolCallsTotal.WithLabelValues(step, tool, status).Inc()
}

func RecordToolRound(step string) {
	ToolRoundsTotal.WithLabelValues(step).Inc()
}

func RecordToolRoundDuration(step string, d time.Duration) {
	ToolRoundDurationSeconds.WithLabelValues(step).Observe(d.Seconds())
}

func RecordLLMCacheHit() {
	LLMCacheHitsTotal.Inc()
}

func SetParallelStepsInFlight(n int) {
	ParallelStepsInFlight.Set(float64(n))
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/metrics/ -v`
Expected: All pass.

- [ ] **Step 5: Commit**

```bash
git add internal/metrics/metrics.go internal/metrics/metrics_test.go
git commit -m "feat(phase8): add Prometheus metrics for tool use and parallel execution"
```

---

## Task 12: Logs Output with Sub-Step Tree

**Files:**
- Modify: `internal/cli/logs.go`

- [ ] **Step 1: Add sub-step query and tree rendering**

In `internal/cli/logs.go`, extend the logs query to include sub-steps (rows where `parent_step_id IS NOT NULL`) and render them indented under their parent:

```go
// After fetching top-level steps, fetch sub-steps
subRows, err := db.QueryContext(ctx, `
	SELECT step_name, status, started_at, completed_at, parent_step_id
	FROM step_executions
	WHERE execution_id = $1 AND parent_step_id IS NOT NULL
	ORDER BY created_at
`, executionID)
```

Group sub-steps by parent and render:
```
  ✓ research_agent      completed  8.4s
    ├─ round 1
    │  ├─ get_weather    completed  0.8s
    │  └─ get_forecast   completed  1.1s
    └─ round 2
       └─ get_weather    completed  0.6s
```

Parse round number from step name pattern `parent/tool/name/round`.

- [ ] **Step 2: Run existing logs tests**

Run: `go test ./internal/cli/ -v`
Expected: Existing tests still pass (no sub-steps in existing test data).

- [ ] **Step 3: Commit**

```bash
git add internal/cli/logs.go
git commit -m "feat(phase8): render tool-use sub-steps as tree in mantle logs output"
```

---

## Task 13: Run Full Test Suite

- [ ] **Step 1: Run all tests**

Run: `make test`
Expected: All tests pass.

- [ ] **Step 2: Run linter**

Run: `make lint`
Expected: No new lint errors.

- [ ] **Step 3: Final commit if any cleanup needed**

```bash
git add -A
git commit -m "chore(phase8): final cleanup after full test suite"
```
