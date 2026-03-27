package engine

import (
	"context"
	"database/sql"
	"fmt"
	"hash/fnv"

	"github.com/dvflw/mantle/internal/auth"
	"github.com/dvflw/mantle/internal/metrics"
)

// ConcurrencyResult describes whether an execution is allowed to start,
// should be queued, or was rejected.
type ConcurrencyResult struct {
	Allowed bool
	Queued  bool
	Err     error
}

// CheckConcurrencyLimits evaluates per-team and per-workflow concurrency
// limits within the given transaction. Advisory locks ensure serialisation
// so that two concurrent callers cannot both "win" the last slot.
//
// Evaluation order:
//  1. Per-team limit (teamMaxConcurrent, from engine config / teams table)
//  2. Per-workflow limit (maxParallelExecutions, from workflow YAML)
//
// onLimit controls what happens when a limit is hit: "queue" (default) or
// "reject".
func CheckConcurrencyLimits(ctx context.Context, tx *sql.Tx, workflowName string, maxParallelExecutions int, onLimit string, teamMaxConcurrent int) ConcurrencyResult {
	if onLimit == "" {
		onLimit = "queue"
	}

	teamID := auth.TeamIDFromContext(ctx)

	// --- Per-team check ---
	if teamMaxConcurrent > 0 && teamID != "" {
		// Acquire transaction-scoped advisory lock keyed on team.
		lockKey := hashString("team:" + teamID)
		if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", int64(lockKey)); err != nil {
			return ConcurrencyResult{Err: fmt.Errorf("acquiring team advisory lock: %w", err)}
		}

		var count int
		err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM workflow_executions WHERE team_id = $1 AND status IN ('pending', 'running')`,
			teamID,
		).Scan(&count)
		if err != nil {
			return ConcurrencyResult{Err: fmt.Errorf("counting team executions: %w", err)}
		}

		if count >= teamMaxConcurrent {
			if onLimit == "reject" {
				metrics.ExecutionsRejectedTotal.WithLabelValues(workflowName).Inc()
				return ConcurrencyResult{Err: fmt.Errorf("team %q has reached the maximum of %d concurrent executions", teamID, teamMaxConcurrent)}
			}
			metrics.ExecutionsQueued.WithLabelValues(workflowName).Inc()
			return ConcurrencyResult{Queued: true}
		}
	}

	// --- Per-workflow check ---
	if maxParallelExecutions > 0 {
		lockKey := hashString("workflow:" + workflowName)
		if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", int64(lockKey)); err != nil {
			return ConcurrencyResult{Err: fmt.Errorf("acquiring workflow advisory lock: %w", err)}
		}

		var count int
		err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM workflow_executions WHERE workflow_name = $1 AND status IN ('pending', 'running')`,
			workflowName,
		).Scan(&count)
		if err != nil {
			return ConcurrencyResult{Err: fmt.Errorf("counting workflow executions: %w", err)}
		}

		if count >= maxParallelExecutions {
			if onLimit == "reject" {
				metrics.ExecutionsRejectedTotal.WithLabelValues(workflowName).Inc()
				return ConcurrencyResult{Err: fmt.Errorf("workflow %q has reached the maximum of %d concurrent executions", workflowName, maxParallelExecutions)}
			}
			metrics.ExecutionsQueued.WithLabelValues(workflowName).Inc()
			return ConcurrencyResult{Queued: true}
		}
	}

	return ConcurrencyResult{Allowed: true}
}

// PromoteQueued picks the oldest queued execution for a workflow and
// promotes it to pending so it will be picked up by the engine.
// Uses FOR UPDATE SKIP LOCKED to avoid contention with other promoters.
func PromoteQueued(ctx context.Context, db *sql.DB, workflowName string) error {
	result, err := db.ExecContext(ctx,
		`UPDATE workflow_executions SET status = 'pending', updated_at = NOW()
		 WHERE id = (
		   SELECT id FROM workflow_executions
		   WHERE workflow_name = $1 AND status = 'queued'
		   ORDER BY started_at ASC
		   LIMIT 1
		   FOR UPDATE SKIP LOCKED
		 )`,
		workflowName,
	)
	if err != nil {
		return fmt.Errorf("promoting queued execution: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows > 0 {
		metrics.ExecutionsQueued.WithLabelValues(workflowName).Dec()
	}
	return nil
}

// hashString returns a deterministic uint32 hash for use as an advisory lock key.
func hashString(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}
