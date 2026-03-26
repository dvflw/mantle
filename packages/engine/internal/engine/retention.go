package engine

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

// CleanupExecutions deletes execution data older than the specified retention period.
// Returns the number of workflow execution rows deleted.
// If retentionDays <= 0, cleanup is skipped (disabled).
func CleanupExecutions(ctx context.Context, db *sql.DB, retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		return 0, nil
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	// Delete step executions first (FK constraint).
	result, err := db.ExecContext(ctx,
		`DELETE FROM step_executions WHERE execution_id IN (
			SELECT id FROM workflow_executions
			WHERE completed_at IS NOT NULL AND completed_at < $1
		)`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("cleaning step executions: %w", err)
	}
	stepRows, _ := result.RowsAffected()

	// Delete workflow executions.
	result, err = db.ExecContext(ctx,
		`DELETE FROM workflow_executions
		 WHERE completed_at IS NOT NULL AND completed_at < $1`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("cleaning workflow executions: %w", err)
	}
	execRows, _ := result.RowsAffected()

	if execRows > 0 {
		slog.Info("retention cleanup", "executions_deleted", execRows, "steps_deleted", stepRows, "cutoff", cutoff)
	}
	return execRows, nil
}

// CleanupAuditEvents deletes audit events older than the specified retention period.
// Returns the number of rows deleted.
// If retentionDays <= 0, cleanup is skipped (disabled).
func CleanupAuditEvents(ctx context.Context, db *sql.DB, retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		return 0, nil
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	result, err := db.ExecContext(ctx,
		`DELETE FROM audit_events WHERE timestamp < $1`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("cleaning audit events: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows > 0 {
		slog.Info("audit retention cleanup", "events_deleted", rows, "cutoff", cutoff)
	}
	return rows, nil
}
