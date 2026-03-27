package engine

import (
	"context"
	"database/sql"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/dvflw/mantle/internal/audit"
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
		if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", lockKey); err != nil {
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
		if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", lockKey); err != nil {
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
func PromoteQueued(ctx context.Context, db *sql.DB, workflowName string, auditor audit.Emitter) error {
	var promotedID sql.NullString
	err := db.QueryRowContext(ctx,
		`UPDATE workflow_executions SET status = 'pending', updated_at = NOW()
		 WHERE id = (
		   SELECT id FROM workflow_executions
		   WHERE workflow_name = $1 AND status = 'queued'
		   ORDER BY started_at ASC
		   LIMIT 1
		   FOR UPDATE SKIP LOCKED
		 )
		 RETURNING id`,
		workflowName,
	).Scan(&promotedID)
	if err == sql.ErrNoRows {
		return nil // nothing to promote
	}
	if err != nil {
		return fmt.Errorf("promoting queued execution: %w", err)
	}

	if promotedID.Valid {
		metrics.ExecutionsQueued.WithLabelValues(workflowName).Dec()
		auditor.Emit(ctx, audit.Event{
			Timestamp: time.Now(),
			Actor:     "engine",
			Action:    audit.ActionExecutionPromoted,
			Resource:  audit.Resource{Type: "workflow_execution", ID: promotedID.String},
			Metadata:  map[string]string{"workflow": workflowName, "scope": "workflow"},
		})
	}
	return nil
}

// PromoteQueuedByTeam promotes the oldest queued execution across all workflows for a team.
func PromoteQueuedByTeam(ctx context.Context, db *sql.DB, teamID string, auditor audit.Emitter) error {
	if teamID == "" {
		return nil
	}
	var promotedID sql.NullString
	var promotedWorkflow sql.NullString
	err := db.QueryRowContext(ctx,
		`UPDATE workflow_executions
		 SET status = 'pending', updated_at = NOW()
		 WHERE id = (
		    SELECT id FROM workflow_executions
		    WHERE team_id = $1 AND status = 'queued'
		    ORDER BY started_at ASC
		    LIMIT 1
		    FOR UPDATE SKIP LOCKED
		 )
		 RETURNING id, workflow_name`,
		teamID,
	).Scan(&promotedID, &promotedWorkflow)
	if err == sql.ErrNoRows {
		return nil // nothing to promote
	}
	if err != nil {
		return fmt.Errorf("promoting queued execution for team: %w", err)
	}

	if promotedID.Valid {
		wfName := promotedWorkflow.String
		metrics.ExecutionsQueued.WithLabelValues(wfName).Dec()
		auditor.Emit(ctx, audit.Event{
			Timestamp: time.Now(),
			Actor:     "engine",
			Action:    audit.ActionExecutionPromoted,
			Resource:  audit.Resource{Type: "workflow_execution", ID: promotedID.String},
			Metadata:  map[string]string{"workflow": wfName, "team_id": teamID, "scope": "team"},
		})
	}
	return nil
}

// hashString returns a deterministic int64 hash for use as a Postgres advisory lock key.
func hashString(s string) int64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return int64(h.Sum64())
}
