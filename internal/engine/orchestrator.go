package engine

import (
	"context"
	"database/sql"
	"fmt"
)

// StepStatus holds the current status and attempt number for a step execution.
type StepStatus struct {
	Status  string
	Attempt int
}

// Orchestrator coordinates step execution against the database.
type Orchestrator struct {
	db *sql.DB
}

// NewOrchestrator creates a new Orchestrator backed by the given database.
func NewOrchestrator(db *sql.DB) *Orchestrator {
	return &Orchestrator{db: db}
}

// CreatePendingSteps inserts pending step_executions rows for the given step
// names under the specified execution ID. The attemptOverrides map allows
// setting a specific attempt number per step; steps not in the map default to 1.
func (o *Orchestrator) CreatePendingSteps(ctx context.Context, executionID string, stepNames []string, attemptOverrides map[string]int) error {
	tx, err := o.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	for _, name := range stepNames {
		attempt := 1
		if override, ok := attemptOverrides[name]; ok {
			attempt = override
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO step_executions (execution_id, step_name, attempt, status)
			 VALUES ($1, $2, $3, 'pending')`,
			executionID, name, attempt,
		)
		if err != nil {
			return fmt.Errorf("insert step %q: %w", name, err)
		}
	}

	return tx.Commit()
}

// GetStepStatuses returns the current status and latest attempt for each step
// in the given execution. Only the row with the highest attempt per step is
// returned.
func (o *Orchestrator) GetStepStatuses(ctx context.Context, executionID string) (map[string]StepStatus, error) {
	rows, err := o.db.QueryContext(ctx,
		`SELECT DISTINCT ON (step_name) step_name, status, attempt
		 FROM step_executions
		 WHERE execution_id = $1
		 ORDER BY step_name, attempt DESC`,
		executionID,
	)
	if err != nil {
		return nil, fmt.Errorf("query step statuses: %w", err)
	}
	defer rows.Close()

	statuses := make(map[string]StepStatus)
	for rows.Next() {
		var name, status string
		var attempt int
		if err := rows.Scan(&name, &status, &attempt); err != nil {
			return nil, fmt.Errorf("scan step status: %w", err)
		}
		statuses[name] = StepStatus{Status: status, Attempt: attempt}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate step statuses: %w", err)
	}
	return statuses, nil
}

// CancelPendingSteps sets the status of all pending step_executions for the
// given step names to "cancelled".
func (o *Orchestrator) CancelPendingSteps(ctx context.Context, executionID string, stepNames []string) error {
	if len(stepNames) == 0 {
		return nil
	}

	tx, err := o.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	for _, name := range stepNames {
		_, err := tx.ExecContext(ctx,
			`UPDATE step_executions
			 SET status = 'cancelled', updated_at = NOW()
			 WHERE execution_id = $1 AND step_name = $2 AND status = 'pending'`,
			executionID, name,
		)
		if err != nil {
			return fmt.Errorf("cancel step %q: %w", name, err)
		}
	}

	return tx.Commit()
}
