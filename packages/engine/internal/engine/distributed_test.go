package engine

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDistributed_NoStepDuplication(t *testing.T) {
	db := setupTestDB(t)
	execID := createTestExecution(t, db)

	// Create 10 pending steps.
	for i := 0; i < 10; i++ {
		insertPendingStep(t, db, execID, fmt.Sprintf("step-%d", i), 1)
	}

	// Atomic counter to track total executions across all workers.
	var execCount atomic.Int64

	// Mock executor: atomically increment counter, sleep briefly.
	executor := func(ctx context.Context, stepName string, attempt int) (map[string]any, error) {
		execCount.Add(1)
		time.Sleep(10 * time.Millisecond)
		return map[string]any{"step": stepName}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start 3 workers with unique NodeIDs.
	for i := 0; i < 3; i++ {
		claimer := &Claimer{
			DB:            db,
			NodeID:        fmt.Sprintf("worker-%d", i),
			LeaseDuration: 2 * time.Second,
		}
		worker := &Worker{
			Claimer:            claimer,
			StepExecutor:       executor,
			PollInterval:       50 * time.Millisecond,
			MaxBackoff:         200 * time.Millisecond,
			LeaseRenewInterval: 500 * time.Millisecond,
			Logger:             slog.Default(),
		}
		go worker.Run(ctx)
	}

	// Wait until all 10 steps are completed.
	require.Eventually(t, func() bool {
		var count int
		err := db.QueryRow(
			"SELECT COUNT(*) FROM step_executions WHERE execution_id = $1 AND status = 'completed'",
			execID,
		).Scan(&count)
		if err != nil {
			return false
		}
		return count == 10
	}, 20*time.Second, 100*time.Millisecond, "all 10 steps should reach completed status")

	// Assert exactly 10 executions happened (no duplication).
	require.Equal(t, int64(10), execCount.Load(), "exactly 10 step executions should have occurred")

	// Verify all 10 rows have status='completed'.
	var completedCount int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM step_executions WHERE execution_id = $1 AND status = 'completed'",
		execID,
	).Scan(&completedCount)
	require.NoError(t, err)
	require.Equal(t, 10, completedCount, "all 10 step_executions rows should be completed")

	cancel()
}

func TestDistributed_CrashRecovery(t *testing.T) {
	db := setupTestDB(t)
	execID := createTestExecution(t, db)

	// Create 1 pending step with max_attempts=3.
	var stepID string
	err := db.QueryRow(`
		INSERT INTO step_executions (execution_id, step_name, attempt, status, max_attempts)
		VALUES ($1, 'crash-step', 1, 'pending', 3)
		RETURNING id
	`, execID).Scan(&stepID)
	require.NoError(t, err)

	// Simulate a crashed worker: set to running with expired lease.
	_, err = db.Exec(`
		UPDATE step_executions
		SET status = 'running',
		    claimed_by = 'dead-node',
		    lease_expires_at = NOW() - INTERVAL '1 second',
		    started_at = NOW()
		WHERE id = $1
	`, stepID)
	require.NoError(t, err)

	// Run the reaper to reclaim the step.
	reaper := &Reaper{
		DB:       db,
		Interval: 100 * time.Millisecond,
		Logger:   slog.Default(),
	}
	reaped, err := reaper.ReapSteps(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(1), reaped, "reaper should reclaim 1 step")

	// Verify step is now status='failed' with error='lease expired'.
	var status, stepErr string
	err = db.QueryRow(
		"SELECT status, error FROM step_executions WHERE id = $1", stepID,
	).Scan(&status, &stepErr)
	require.NoError(t, err)
	require.Equal(t, "failed", status)
	require.Equal(t, "lease expired", stepErr)

	// Use orchestrator to create retry (attempt 2).
	orchestrator := &Orchestrator{
		DB:            db,
		NodeID:        "orchestrator-1",
		LeaseDuration: 30 * time.Second,
		PollInterval:  100 * time.Millisecond,
		Logger:        slog.Default(),
	}
	err = orchestrator.CreateRetryStep(context.Background(), execID, "crash-step", 2, 3)
	require.NoError(t, err)

	// Verify retry step exists as pending.
	var retryStatus string
	err = db.QueryRow(
		"SELECT status FROM step_executions WHERE execution_id = $1 AND step_name = 'crash-step' AND attempt = 2",
		execID,
	).Scan(&retryStatus)
	require.NoError(t, err)
	require.Equal(t, "pending", retryStatus)

	// Start a fresh worker to execute the retry.
	var executedAttempt atomic.Int32
	executor := func(ctx context.Context, stepName string, attempt int) (map[string]any, error) {
		executedAttempt.Store(int32(attempt))
		return map[string]any{"recovered": true}, nil
	}

	claimer := &Claimer{
		DB:            db,
		NodeID:        "recovery-worker",
		LeaseDuration: 2 * time.Second,
	}
	worker := &Worker{
		Claimer:            claimer,
		StepExecutor:       executor,
		PollInterval:       50 * time.Millisecond,
		MaxBackoff:         200 * time.Millisecond,
		LeaseRenewInterval: 500 * time.Millisecond,
		Logger:             slog.Default(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go worker.Run(ctx)

	// Wait for retry step to complete.
	require.Eventually(t, func() bool {
		var s string
		err := db.QueryRow(
			"SELECT status FROM step_executions WHERE execution_id = $1 AND step_name = 'crash-step' AND attempt = 2",
			execID,
		).Scan(&s)
		if err != nil {
			return false
		}
		return s == "completed"
	}, 10*time.Second, 100*time.Millisecond, "retry step should complete successfully")

	require.Equal(t, int32(2), executedAttempt.Load(), "executor should have been called with attempt 2")

	cancel()
}

func TestDistributed_MultipleSimultaneousCrashes(t *testing.T) {
	db := setupTestDB(t)
	execID := createTestExecution(t, db)

	// Create 5 pending steps with max_attempts=3.
	stepNames := make([]string, 5)
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("multi-step-%d", i)
		stepNames[i] = name
		_, err := db.Exec(`
			INSERT INTO step_executions (execution_id, step_name, attempt, status, max_attempts)
			VALUES ($1, $2, 1, 'pending', 3)
		`, execID, name)
		require.NoError(t, err)
	}

	var completedCount atomic.Int64
	executor := func(ctx context.Context, stepName string, attempt int) (map[string]any, error) {
		completedCount.Add(1)
		time.Sleep(50 * time.Millisecond) // Simulate some work
		return map[string]any{"step": stepName, "attempt": attempt}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Start 3 workers, each with its own cancellable context.
	workerCtxs := make([]context.Context, 3)
	workerCancels := make([]context.CancelFunc, 3)
	for i := 0; i < 3; i++ {
		workerCtxs[i], workerCancels[i] = context.WithCancel(ctx)
		claimer := &Claimer{
			DB:            db,
			NodeID:        fmt.Sprintf("crash-worker-%d", i),
			LeaseDuration: 2 * time.Second,
		}
		worker := &Worker{
			Claimer:            claimer,
			StepExecutor:       executor,
			PollInterval:       50 * time.Millisecond,
			MaxBackoff:         200 * time.Millisecond,
			LeaseRenewInterval: 500 * time.Millisecond,
			Logger:             slog.Default(),
		}
		go worker.Run(workerCtxs[i])
	}

	// Wait until at least 2 steps complete.
	require.Eventually(t, func() bool {
		return completedCount.Load() >= 2
	}, 15*time.Second, 50*time.Millisecond, "at least 2 steps should complete")

	// Kill workers 0 and 1 to simulate crashes.
	workerCancels[0]()
	workerCancels[1]()
	t.Log("killed workers 0 and 1")

	// Run reaper periodically to reclaim any steps held by dead workers.
	reaper := &Reaper{
		DB:       db,
		Interval: 500 * time.Millisecond,
		Logger:   slog.Default(),
	}
	reaperCtx, reaperCancel := context.WithCancel(ctx)
	defer reaperCancel()
	go reaper.Run(reaperCtx)

	// Run an orchestrator loop that creates retries for failed steps.
	orchestrator := &Orchestrator{
		DB:            db,
		NodeID:        "orchestrator-crash",
		LeaseDuration: 30 * time.Second,
		PollInterval:  200 * time.Millisecond,
		Logger:        slog.Default(),
	}
	orchestratorCtx, orchestratorCancel := context.WithCancel(ctx)
	defer orchestratorCancel()
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-orchestratorCtx.Done():
				return
			case <-ticker.C:
				retryFailedSteps(t, db, orchestrator, execID)
			}
		}
	}()

	// Wait until all 5 steps reach a final completed state (checking latest attempt per step).
	require.Eventually(t, func() bool {
		return allStepsCompleted(db, execID, 5)
	}, 45*time.Second, 200*time.Millisecond, "all 5 steps should eventually complete")

	reaperCancel()
	orchestratorCancel()
	cancel()
}

func TestDistributed_ContinueOnError(t *testing.T) {
	db := setupTestDB(t)
	execID := createTestExecution(t, db)

	// Define a two-step workflow: step-1 has continue_on_error=true and will fail;
	// step-2 depends on step-1 and should still be scheduled and complete.
	steps := []workflowStep{
		{Name: "step-1", ContinueOnError: true},
		{Name: "step-2", DependsOn: []string{"step-1"}},
	}

	orchestrator := &Orchestrator{
		DB:            db,
		NodeID:        "orchestrator-coe",
		LeaseDuration: 30 * time.Second,
		PollInterval:  100 * time.Millisecond,
		Logger:        slog.Default(),
	}

	// Bootstrap: schedule the initial ready steps (step-1 has no deps).
	_, err := orchestrator.AdvanceExecution(context.Background(), execID, steps)
	require.NoError(t, err)

	// Worker executor: step-1 returns an error; step-2 succeeds.
	executor := func(ctx context.Context, stepName string, attempt int) (map[string]any, error) {
		if stepName == "step-1" {
			return nil, fmt.Errorf("deliberate failure")
		}
		return map[string]any{"result": "ok"}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	claimer := &Claimer{
		DB:            db,
		NodeID:        "worker-coe",
		LeaseDuration: 5 * time.Second,
	}
	worker := &Worker{
		Claimer:            claimer,
		StepExecutor:       executor,
		PollInterval:       50 * time.Millisecond,
		MaxBackoff:         200 * time.Millisecond,
		LeaseRenewInterval: 2 * time.Second,
		Logger:             slog.Default(),
	}
	go worker.Run(ctx)

	// Wait for step-1 to reach failed status.
	require.Eventually(t, func() bool {
		var status string
		err := db.QueryRow(
			"SELECT status FROM step_executions WHERE execution_id = $1 AND step_name = 'step-1'",
			execID,
		).Scan(&status)
		return err == nil && status == "failed"
	}, 15*time.Second, 100*time.Millisecond, "step-1 should be marked failed")

	// Advance the orchestrator — step-1 failed with continue_on_error, so step-2 should be scheduled.
	scheduled, err := orchestrator.AdvanceExecution(context.Background(), execID, steps)
	require.NoError(t, err)
	require.Contains(t, scheduled, "step-2", "step-2 should be scheduled after step-1 failed with continue_on_error")

	// Wait for step-2 to complete.
	require.Eventually(t, func() bool {
		var status string
		err := db.QueryRow(
			"SELECT status FROM step_executions WHERE execution_id = $1 AND step_name = 'step-2'",
			execID,
		).Scan(&status)
		return err == nil && status == "completed"
	}, 15*time.Second, 100*time.Millisecond, "step-2 should complete")

	// Verify step-1 is still marked failed (not altered by orchestrator).
	var step1Status string
	require.NoError(t, db.QueryRow(
		"SELECT status FROM step_executions WHERE execution_id = $1 AND step_name = 'step-1'",
		execID,
	).Scan(&step1Status))
	require.Equal(t, "failed", step1Status, "step-1 should remain failed in the DB")

	cancel()
}

// retryFailedSteps checks for failed steps that have remaining attempts and creates retries.
func retryFailedSteps(t *testing.T, db *sql.DB, orch *Orchestrator, execID string) {
	t.Helper()
	rows, err := db.Query(`
		SELECT step_name, attempt, max_attempts
		FROM step_executions
		WHERE execution_id = $1
		  AND status = 'failed'
		  AND parent_step_id IS NULL
	`, execID)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var stepName string
		var attempt, maxAttempts int
		if err := rows.Scan(&stepName, &attempt, &maxAttempts); err != nil {
			continue
		}
		nextAttempt := attempt + 1
		if nextAttempt > maxAttempts {
			continue
		}
		// Check if next attempt already exists.
		var exists bool
		_ = db.QueryRow(
			"SELECT EXISTS(SELECT 1 FROM step_executions WHERE execution_id = $1 AND step_name = $2 AND attempt = $3)",
			execID, stepName, nextAttempt,
		).Scan(&exists)
		if exists {
			continue
		}
		_ = orch.CreateRetryStep(context.Background(), execID, stepName, nextAttempt, maxAttempts)
	}
}

// allStepsCompleted checks if all expected steps have at least one completed attempt.
func allStepsCompleted(db *sql.DB, execID string, expected int) bool {
	var count int
	err := db.QueryRow(`
		SELECT COUNT(DISTINCT step_name) FROM step_executions
		WHERE execution_id = $1
		  AND status = 'completed'
		  AND parent_step_id IS NULL
	`, execID).Scan(&count)
	if err != nil {
		return false
	}
	return count >= expected
}
