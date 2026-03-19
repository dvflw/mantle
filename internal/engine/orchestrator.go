package engine

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

// StepStatus represents the current status of a step execution.
type StepStatus struct {
	Status      string
	Attempt     int
	MaxAttempts int
	Output      map[string]any
	Error       string
}

// Orchestrator coordinates multi-node execution claims and step lifecycle
// using Postgres advisory locks and SKIP LOCKED for distributed coordination.
type Orchestrator struct {
	DB            *sql.DB
	NodeID        string
	LeaseDuration time.Duration
	PollInterval  time.Duration
	Logger        *slog.Logger
}

// ClaimExecution attempts to claim an execution for this node.
// Returns true if the claim was acquired, false if another node already holds it.
// Uses INSERT ... ON CONFLICT DO NOTHING for atomic claim acquisition.
func (o *Orchestrator) ClaimExecution(ctx context.Context, executionID string) (bool, error) {
	leaseExpiry := time.Now().Add(o.LeaseDuration)
	result, err := o.DB.ExecContext(ctx,
		`INSERT INTO execution_claims (execution_id, claimed_by, lease_expires_at)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (execution_id) DO NOTHING`,
		executionID, o.NodeID, leaseExpiry,
	)
	if err != nil {
		return false, fmt.Errorf("claiming execution %s: %w", executionID, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("checking claim result for %s: %w", executionID, err)
	}
	return rows == 1, nil
}

// RenewExecutionLease extends the lease expiry for an execution claimed by this node.
// The update is fenced by claimed_by to prevent a node from renewing another node's lease.
func (o *Orchestrator) RenewExecutionLease(ctx context.Context, executionID string) error {
	newExpiry := time.Now().Add(o.LeaseDuration)
	result, err := o.DB.ExecContext(ctx,
		`UPDATE execution_claims
		 SET lease_expires_at = $1
		 WHERE execution_id = $2 AND claimed_by = $3`,
		newExpiry, executionID, o.NodeID,
	)
	if err != nil {
		return fmt.Errorf("renewing lease for execution %s: %w", executionID, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking renew result for %s: %w", executionID, err)
	}
	if rows == 0 {
		return fmt.Errorf("lease renewal failed for execution %s: not claimed by this node", executionID)
	}
	return nil
}

// ReleaseExecution releases the claim on an execution held by this node.
// The delete is fenced by claimed_by to prevent releasing another node's claim.
func (o *Orchestrator) ReleaseExecution(ctx context.Context, executionID string) error {
	result, err := o.DB.ExecContext(ctx,
		`DELETE FROM execution_claims
		 WHERE execution_id = $1 AND claimed_by = $2`,
		executionID, o.NodeID,
	)
	if err != nil {
		return fmt.Errorf("releasing execution %s: %w", executionID, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking release result for %s: %w", executionID, err)
	}
	if rows == 0 {
		return fmt.Errorf("release failed for execution %s: not claimed by this node", executionID)
	}
	return nil
}

// CreatePendingSteps inserts pending step_execution rows for the given step names.
// Uses ON CONFLICT DO NOTHING for idempotent creation (safe to call multiple times).
func (o *Orchestrator) CreatePendingSteps(ctx context.Context, executionID string, stepNames []string, maxAttempts map[string]int) error {
	tx, err := o.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO step_executions (execution_id, step_name, attempt, status, max_attempts)
		 VALUES ($1, $2, 1, 'pending', $3)
		 ON CONFLICT (execution_id, step_name, attempt) DO NOTHING`,
	)
	if err != nil {
		return fmt.Errorf("preparing statement: %w", err)
	}
	defer stmt.Close()

	for _, name := range stepNames {
		ma := 1
		if v, ok := maxAttempts[name]; ok {
			ma = v
		}
		if _, err := stmt.ExecContext(ctx, executionID, name, ma); err != nil {
			return fmt.Errorf("inserting step %s: %w", name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing pending steps: %w", err)
	}
	return nil
}

// CreateRetryStep creates a new attempt row for a step that needs to be retried.
func (o *Orchestrator) CreateRetryStep(ctx context.Context, executionID, stepName string, nextAttempt, maxAttempts int) error {
	_, err := o.DB.ExecContext(ctx,
		`INSERT INTO step_executions (execution_id, step_name, attempt, status, max_attempts)
		 VALUES ($1, $2, $3, 'pending', $4)
		 ON CONFLICT (execution_id, step_name, attempt) DO NOTHING`,
		executionID, stepName, nextAttempt, maxAttempts,
	)
	if err != nil {
		return fmt.Errorf("creating retry step %s attempt %d: %w", stepName, nextAttempt, err)
	}
	return nil
}

// GetStepStatuses returns the latest status for each top-level step in an execution.
// Uses DISTINCT ON to return only the most recent attempt per step name.
func (o *Orchestrator) GetStepStatuses(ctx context.Context, executionID string) (map[string]StepStatus, error) {
	rows, err := o.DB.QueryContext(ctx,
		`SELECT DISTINCT ON (step_name) step_name, status, attempt, max_attempts, output, error
		 FROM step_executions
		 WHERE execution_id = $1 AND parent_step_id IS NULL
		 ORDER BY step_name, attempt DESC`,
		executionID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying step statuses for %s: %w", executionID, err)
	}
	defer rows.Close()

	statuses := make(map[string]StepStatus)
	for rows.Next() {
		var (
			name       string
			s          StepStatus
			outputJSON sql.NullString
			errStr     sql.NullString
		)
		if err := rows.Scan(&name, &s.Status, &s.Attempt, &s.MaxAttempts, &outputJSON, &errStr); err != nil {
			return nil, fmt.Errorf("scanning step status: %w", err)
		}
		if errStr.Valid {
			s.Error = errStr.String
		}
		// Output parsing is left to callers that need it; we store the raw status here.
		statuses[name] = s
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating step statuses: %w", err)
	}
	return statuses, nil
}

// CancelPendingSteps sets all pending steps with the given names to cancelled status.
func (o *Orchestrator) CancelPendingSteps(ctx context.Context, executionID string, stepNames []string) error {
	if len(stepNames) == 0 {
		return nil
	}

	tx, err := o.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`UPDATE step_executions
		 SET status = 'cancelled', updated_at = NOW()
		 WHERE execution_id = $1 AND status = 'pending' AND step_name = $2`,
	)
	if err != nil {
		return fmt.Errorf("preparing cancel statement: %w", err)
	}
	defer stmt.Close()

	for _, name := range stepNames {
		if _, err := stmt.ExecContext(ctx, executionID, name); err != nil {
			return fmt.Errorf("cancelling step %s: %w", name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing cancel: %w", err)
	}
	return nil
}
