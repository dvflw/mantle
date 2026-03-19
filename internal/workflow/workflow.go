package workflow

import "fmt"

// Workflow represents a complete workflow definition parsed from YAML.
type Workflow struct {
	Name        string           `yaml:"name"`
	Description string           `yaml:"description"`
	Inputs      map[string]Input `yaml:"inputs"`
	Steps       []Step           `yaml:"steps"`
}

// Input defines a workflow input parameter.
type Input struct {
	Type        string `yaml:"type"`
	Description string `yaml:"description"`
}

// Step defines a single step within a workflow.
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

// ValidationError represents a structural validation error with source location.
type ValidationError struct {
	Line    int
	Column  int
	Field   string
	Message string
}

// Error formats the validation error as "line:col: error: message (field)".
func (e ValidationError) Error() string {
	return fmt.Sprintf("%d:%d: error: %s (%s)", e.Line, e.Column, e.Message, e.Field)
}

// Tool represents a tool definition for AI tool-use steps.
type Tool struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	InputSchema map[string]any `yaml:"input_schema"`
	Action      string         `yaml:"action"`
	Params      map[string]any `yaml:"params"`
}

// ParseTools extracts tool definitions from a step's params map.
// It expects params to contain a "tools" key with a list of tool objects.
func ParseTools(params map[string]any) ([]Tool, error) {
	raw, ok := params["tools"]
	if !ok {
		return nil, nil
	}

	toolList, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("tools must be a list")
	}

	tools := make([]Tool, 0, len(toolList))
	for i, item := range toolList {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("tools[%d]: expected a mapping", i)
		}

		var t Tool
		if v, ok := m["name"].(string); ok {
			t.Name = v
		}
		if v, ok := m["description"].(string); ok {
			t.Description = v
		}
		if v, ok := m["input_schema"].(map[string]any); ok {
			t.InputSchema = v
		}
		if v, ok := m["action"].(string); ok {
			t.Action = v
		}
		if v, ok := m["params"].(map[string]any); ok {
			t.Params = v
		}

		tools = append(tools, t)
	}

	return tools, nil
}
