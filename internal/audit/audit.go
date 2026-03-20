package audit

import (
	"context"
	"time"
)

// AuthMethodExtractor is a function that extracts the auth method from context.
// Set this at startup to avoid import cycles between audit and auth packages.
var AuthMethodExtractor func(ctx context.Context) string

// Action represents the type of state-changing operation that occurred.
type Action string

const (
	ActionWorkflowApplied    Action = "workflow.applied"
	ActionWorkflowExecuted   Action = "workflow.executed"
	ActionStepStarted        Action = "step.started"
	ActionStepCompleted      Action = "step.completed"
	ActionStepFailed         Action = "step.failed"
	ActionStepSkipped        Action = "step.skipped"
	ActionExecutionCancelled Action = "execution.cancelled"
)

// Resource identifies the target of an audit event.
type Resource struct {
	Type string // e.g., "workflow_definition", "workflow_execution", "step_execution"
	ID   string
}

// Event represents a single audit event emitted by a state-changing operation.
type Event struct {
	ID        string
	Timestamp time.Time
	Actor     string            // who performed the action (e.g., "cli", "scheduler", user ID)
	Action    Action            // what happened
	Resource  Resource          // what was affected
	Before    any               // optional: state before the change
	After     any               // optional: state after the change
	Metadata  map[string]string // optional: additional context
}

// Emitter is the interface for emitting audit events. Implementations may
// discard events (no-op), log them, or persist them to a database.
type Emitter interface {
	Emit(ctx context.Context, event Event) error
}

// enrichFromContext adds contextual metadata to an event.
// Currently extracts auth_method from context via the registered extractor.
func enrichFromContext(ctx context.Context, event Event) Event {
	if AuthMethodExtractor == nil {
		return event
	}
	if method := AuthMethodExtractor(ctx); method != "" {
		if event.Metadata == nil {
			event.Metadata = make(map[string]string)
		}
		event.Metadata["auth_method"] = method
	}
	return event
}
