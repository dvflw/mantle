package engine

// StepStatus represents the current state of a step execution.
type StepStatus struct {
	Status      string
	Attempt     int
	MaxAttempts int
	Output      map[string]any
	Error       string
}
