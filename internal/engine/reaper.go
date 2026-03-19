package engine

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/dvflw/mantle/internal/metrics"
)

// Reaper periodically reclaims step executions and execution claims
// whose leases have expired. This ensures that work held by crashed
// or partitioned nodes is eventually released.
type Reaper struct {
	DB       *sql.DB
	Interval time.Duration
	Logger   *slog.Logger
}

// ReapSteps marks running steps with expired leases as failed.
// The orchestrator is responsible for creating new retry rows if the
// step has remaining attempts. Returns the number of reclaimed steps.
func (r *Reaper) ReapSteps(ctx context.Context) (int64, error) {
	result, err := r.DB.ExecContext(ctx, `
		UPDATE step_executions
		SET status = 'failed', error = 'lease expired',
		    completed_at = NOW(), claimed_by = NULL, lease_expires_at = NULL,
		    updated_at = NOW()
		WHERE status = 'running' AND lease_expires_at < NOW()
	`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ReapExecutionClaims deletes expired execution claims, allowing other
// nodes to pick up the orphaned executions. Returns the number of
// deleted claims.
func (r *Reaper) ReapExecutionClaims(ctx context.Context) (int64, error) {
	result, err := r.DB.ExecContext(ctx, `
		DELETE FROM execution_claims WHERE lease_expires_at < NOW()
	`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// Run starts the reaper loop. It blocks until ctx is cancelled.
// Step reaping runs on every tick. Execution claim reaping is offset
// by half the interval to spread database load.
func (r *Reaper) Run(ctx context.Context) {
	ticker := time.NewTicker(r.Interval)
	defer ticker.Stop()

	// Offset the execution claim reaper by half the interval.
	halfInterval := r.Interval / 2
	claimTimer := time.NewTimer(halfInterval)
	defer claimTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := r.ReapSteps(ctx)
			if err != nil {
				r.Logger.Error("reaper: failed to reap steps", "error", err)
			} else if n > 0 {
				r.Logger.Info("reaper: reclaimed expired step leases", "count", n)
				metrics.RecordReaperReclaimed(int(n))
				for range n {
					metrics.RecordLeaseExpiration()
				}
			}

			// Reset the claim timer for the next offset cycle.
			claimTimer.Reset(halfInterval)
		case <-claimTimer.C:
			n, err := r.ReapExecutionClaims(ctx)
			if err != nil {
				r.Logger.Error("reaper: failed to reap execution claims", "error", err)
			} else if n > 0 {
				r.Logger.Info("reaper: released expired execution claims", "count", n)
			}
		}
	}
}
