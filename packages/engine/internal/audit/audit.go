package audit

import (
	"context"
	"time"
)

// Action represents the type of state-changing operation that occurred.
type Action string

const (
	ActionWorkflowApplied    Action = "workflow.applied"
	ActionWorkflowExecuted   Action = "workflow.executed"
	ActionStepStarted        Action = "step.started"
	ActionStepCompleted      Action = "step.completed"
	ActionStepFailed         Action = "step.failed"
	ActionStepSkipped           Action = "step.skipped"
	ActionStepContinuedOnError  Action = "step.continued_on_error"
	ActionExecutionCancelled    Action = "execution.cancelled"
	ActionExecutionRetried      Action = "execution.retried"
	ActionArtifactPersisted  Action = "artifact.persisted"

	// Admin operations.
	ActionUserCreated       Action = "user.created"
	ActionUserDeleted       Action = "user.deleted"
	ActionUserRoleChanged   Action = "user.role_changed"
	ActionTeamCreated       Action = "team.created"
	ActionTeamDeleted       Action = "team.deleted"
	ActionAPIKeyCreated     Action = "apikey.created"
	ActionAPIKeyRevoked     Action = "apikey.revoked"
	ActionCredentialCreated Action = "credential.created"
	ActionCredentialDeleted Action = "credential.deleted"
	ActionCredentialRotated  Action = "credential.rotated"
	ActionSecretKeyRotated   Action = "secret.key_rotated"
	ActionAuthFailed        Action = "auth.failed"

	// Email trigger operations.
	ActionEmailTriggerFired          Action = "email.trigger.fired"
	ActionEmailConnectionEstablished Action = "email.connection.established"
	ActionEmailConnectionFailed      Action = "email.connection.failed"

	// Hook operations.
	ActionHookStepStarted   Action = "hook.step.started"
	ActionHookStepCompleted Action = "hook.step.completed"
	ActionHookStepFailed    Action = "hook.step.failed"

	// Budget operations.
	ActionBudgetExceeded Action = "budget.exceeded"
	ActionBudgetWarning  Action = "budget.warning"
	ActionBudgetUpdated  Action = "budget.updated"
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
	TeamID    string            // optional: team scope for multi-tenant audit queries
}

// Emitter is the interface for emitting audit events. Implementations may
// discard events (no-op), log them, or persist them to a database.
type Emitter interface {
	Emit(ctx context.Context, event Event) error
}

