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
	Name       string         `yaml:"name"`
	Action     string         `yaml:"action"`
	Params     map[string]any `yaml:"params"`
	If         string         `yaml:"if"`
	Retry      *RetryPolicy   `yaml:"retry"`
	Timeout    string         `yaml:"timeout"`
	Credential string         `yaml:"credential"`
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
