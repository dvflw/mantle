package workflow

import "fmt"

// Workflow represents a complete workflow definition parsed from YAML.
type Workflow struct {
	Name        string           `yaml:"name"`
	Description string           `yaml:"description"`
	Inputs      map[string]Input `yaml:"inputs"`
	Triggers    []Trigger        `yaml:"triggers"`
	Steps       []Step           `yaml:"steps"`
}

// Trigger defines an automatic execution trigger for a workflow.
type Trigger struct {
	Type     string `yaml:"type"`     // "cron" or "webhook"
	Schedule string `yaml:"schedule"` // cron expression (for type: cron)
	Path     string `yaml:"path"`     // webhook path (for type: webhook)
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
	DependsOn  []string       `yaml:"depends_on"`
}

// FindStep returns a pointer to the step with the given name, or nil if not found.
func (w *Workflow) FindStep(name string) *Step {
	for i := range w.Steps {
		if w.Steps[i].Name == name {
			return &w.Steps[i]
		}
	}
	return nil
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
