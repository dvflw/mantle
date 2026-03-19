package engine

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// StepClaim represents a successfully claimed step execution.
type StepClaim struct {
	ID        string
	StepName  string
	Attempt   int
	ClaimedBy string
}

// Claimer provides methods to claim, complete, and manage leases on step
// executions using Postgres advisory locking (SKIP LOCKED) and fencing tokens.
type Claimer struct {
	DB            *sql.DB
	NodeID        string
	LeaseDuration time.Duration
}

// ClaimStep attempts to claim a pending step for a specific execution using
// SELECT ... FOR UPDATE SKIP LOCKED. It returns nil if no pending step is
// available. The claim and execution happen in separate transactions to avoid
// holding row locks during step execution.
func (c *Claimer) ClaimStep(ctx context.Context, executionID string) (*StepClaim, error) {
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	interval := fmt.Sprintf("%d seconds", int(c.LeaseDuration.Seconds()))

	var claim StepClaim
	err = tx.QueryRowContext(ctx, `
		SELECT id, step_name, attempt
		FROM step_executions
		WHERE execution_id = $1
		  AND status = 'pending'
		  AND claimed_by IS NULL
		ORDER BY created_at ASC
		LIMIT 1
		FOR UPDATE SKIP LOCKED
	`, executionID).Scan(&claim.ID, &claim.StepName, &claim.Attempt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("select pending step: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE step_executions
		SET status = 'running',
		    claimed_by = $1,
		    lease_expires_at = NOW() + ($2 || ' seconds')::interval,
		    started_at = NOW(),
		    updated_at = NOW()
		WHERE id = $3
	`, c.NodeID, interval, claim.ID)
	if err != nil {
		return nil, fmt.Errorf("update step claim: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit claim: %w", err)
	}

	claim.ClaimedBy = c.NodeID
	return &claim, nil
}

// ClaimAnyStep attempts to claim a pending step across all executions.
// Sub-steps (those with a non-null parent_step_id) are excluded.
// Returns the claim and the execution ID, or nil if no work is available.
func (c *Claimer) ClaimAnyStep(ctx context.Context) (*StepClaim, string, error) {
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	interval := fmt.Sprintf("%d seconds", int(c.LeaseDuration.Seconds()))

	var claim StepClaim
	var executionID string
	err = tx.QueryRowContext(ctx, `
		SELECT id, execution_id, step_name, attempt
		FROM step_executions
		WHERE status = 'pending'
		  AND claimed_by IS NULL
		  AND parent_step_id IS NULL
		ORDER BY created_at ASC
		LIMIT 1
		FOR UPDATE SKIP LOCKED
	`).Scan(&claim.ID, &executionID, &claim.StepName, &claim.Attempt)
	if err == sql.ErrNoRows {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("select pending step: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE step_executions
		SET status = 'running',
		    claimed_by = $1,
		    lease_expires_at = NOW() + ($2 || ' seconds')::interval,
		    started_at = NOW(),
		    updated_at = NOW()
		WHERE id = $3
	`, c.NodeID, interval, claim.ID)
	if err != nil {
		return nil, "", fmt.Errorf("update step claim: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, "", fmt.Errorf("commit claim: %w", err)
	}

	claim.ClaimedBy = c.NodeID
	return &claim, executionID, nil
}

// CompleteStep marks a step as completed or failed. It uses a fencing token
// pattern: only the node that currently holds the claim can complete the step.
// Returns false if the fencing check rejects the update (e.g., the lease was
// reclaimed by a reaper and given to another worker).
func (c *Claimer) CompleteStep(ctx context.Context, stepID string, output []byte, stepErr error) (bool, error) {
	var errStr *string
	status := "completed"
	if stepErr != nil {
		status = "failed"
		s := stepErr.Error()
		errStr = &s
	}

	result, err := c.DB.ExecContext(ctx, `
		UPDATE step_executions
		SET status = $1,
		    output = $2,
		    error = $3,
<<<<<<< HEAD
=======
		    lease_expires_at = NULL,
>>>>>>> worktree-agent-a6c97b7c
		    completed_at = NOW(),
		    updated_at = NOW()
		WHERE id = $4
		  AND claimed_by = $5
		  AND status = 'running'
	`, status, output, errStr, stepID, c.NodeID)
	if err != nil {
		return false, fmt.Errorf("complete step: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}
	return rows > 0, nil
}

// RenewLease extends the lease on a step execution. Returns false if the
// lease was already reclaimed by another node (fencing check).
func (c *Claimer) RenewLease(ctx context.Context, stepID string) (bool, error) {
	interval := fmt.Sprintf("%d seconds", int(c.LeaseDuration.Seconds()))

	result, err := c.DB.ExecContext(ctx, `
		UPDATE step_executions
		SET lease_expires_at = NOW() + ($1 || ' seconds')::interval,
		    updated_at = NOW()
		WHERE id = $2
		  AND claimed_by = $3
		  AND status = 'running'
	`, interval, stepID, c.NodeID)
	if err != nil {
		return false, fmt.Errorf("renew lease: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}
	return rows > 0, nil
}
