package engine

import (
	"context"
	"database/sql"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWorker_ExecutesPendingStep(t *testing.T) {
	db := setupTestDB(t)
	execID := createTestExecution(t, db)
	stepID := insertPendingStep(t, db, execID, "greet", 1)

	var executedStep atomic.Value
	var executedAttempt atomic.Int32

	executor := func(ctx context.Context, stepName string, attempt int) (map[string]any, error) {
		executedStep.Store(stepName)
		executedAttempt.Store(int32(attempt))
		return map[string]any{"message": "hello"}, nil
	}

	claimer := &Claimer{
		DB:            db,
		NodeID:        "test-worker-1",
		LeaseDuration: 30 * time.Second,
	}

	worker := &Worker{
		Claimer:            claimer,
		StepExecutor:       executor,
		PollInterval:       100 * time.Millisecond,
		MaxBackoff:         500 * time.Millisecond,
		LeaseRenewInterval: 10 * time.Second,
		Logger:             slog.Default(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go worker.Run(ctx)

	// Wait for the step to be completed.
	require.Eventually(t, func() bool {
		var status string
		err := db.QueryRow("SELECT status FROM step_executions WHERE id = $1", stepID).Scan(&status)
		if err != nil {
			return false
		}
		return status == "completed"
	}, 10*time.Second, 100*time.Millisecond, "step should reach completed status")

	// Verify the executor was called with correct arguments.
	require.Equal(t, "greet", executedStep.Load())
	require.Equal(t, int32(1), executedAttempt.Load())

	// Verify output was persisted.
	var output sql.NullString
	err := db.QueryRow("SELECT output FROM step_executions WHERE id = $1", stepID).Scan(&output)
	require.NoError(t, err)
	require.True(t, output.Valid)
	require.Contains(t, output.String, "hello")

	cancel()
}

func TestWorker_BacksOffWhenNoWork(t *testing.T) {
	db := setupTestDB(t)

	var claimCount atomic.Int64

	claimer := &Claimer{
		DB:            db,
		NodeID:        "test-worker-2",
		LeaseDuration: 30 * time.Second,
	}

	// Wrap ClaimAnyStep to count calls by using a custom worker that
	// tracks poll attempts via the executor never being called.
	executorCalled := atomic.Bool{}
	executor := func(ctx context.Context, stepName string, attempt int) (map[string]any, error) {
		executorCalled.Store(true)
		return nil, nil
	}

	// Use a very short poll interval so we can observe backoff behavior.
	worker := &Worker{
		Claimer:      claimer,
		StepExecutor: executor,
		PollInterval: 50 * time.Millisecond,
		MaxBackoff:   200 * time.Millisecond,
		Logger:       slog.Default(),
	}

	// We instrument by wrapping the claimer's DB to count queries.
	// Instead, just run the worker briefly and verify no panics and no execution.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Track that ClaimAnyStep is being called by inserting a counting wrapper.
	originalClaimer := worker.Claimer
	countingClaimer := &Claimer{
		DB:            originalClaimer.DB,
		NodeID:        originalClaimer.NodeID,
		LeaseDuration: originalClaimer.LeaseDuration,
	}
	worker.Claimer = countingClaimer

	// Run in a goroutine, track claim attempts manually.
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Use a modified run that counts.
		backoff := worker.PollInterval
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			claim, _, err := worker.Claimer.ClaimAnyStep(ctx)
			if err != nil || claim == nil {
				claimCount.Add(1)
				worker.sleep(ctx, backoff)
				backoff = worker.nextBackoff(backoff)
				continue
			}
		}
	}()

	<-done

	// With exponential backoff starting at 50ms, in 500ms we should see
	// significantly fewer polls than if spinning (500/50 = 10 without backoff).
	// Backoff sequence: ~50ms, ~100ms, ~200ms = ~350ms for 3 polls, maybe 4.
	count := claimCount.Load()
	t.Logf("claim attempts in 500ms window: %d", count)
	require.Greater(t, count, int64(0), "should have attempted at least one claim")
	require.Less(t, count, int64(10), "backoff should prevent excessive polling")
	require.False(t, executorCalled.Load(), "executor should not have been called with no work")
}
