package engine

import (
	"context"
	"testing"
	"time"

	"github.com/dvflw/mantle/internal/workflow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)


func TestOrchestrator_DAGBasedStepCreation(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()
	o := &Orchestrator{
		DB:            database,
		NodeID:        "test-node",
		LeaseDuration: 120 * time.Second,
	}

	execID := createTestExecution(t, database)

	steps := []workflow.Step{
		{Name: "a"},
		{Name: "b"},
		{Name: "c", DependsOn: []string{"a", "b"}},
	}

	dag, err := BuildDAG(steps)
	require.NoError(t, err)

	// Initially, a and b should be ready (no deps).
	ready := dag.ReadySteps(map[string]string{})
	assert.ElementsMatch(t, []string{"a", "b"}, ready)

	err = o.CreatePendingSteps(ctx, execID, ready, map[string]int{})
	require.NoError(t, err)

	// Verify 2 pending steps created.
	var count int
	err = database.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM step_executions WHERE execution_id = $1 AND status = 'pending'`,
		execID,
	).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	// Simulate a and b completed.
	_, err = database.ExecContext(ctx,
		`UPDATE step_executions SET status = 'completed' WHERE execution_id = $1`,
		execID,
	)
	require.NoError(t, err)

	// Get statuses and check what's ready next.
	statuses, err := o.GetStepStatuses(ctx, execID)
	require.NoError(t, err)

	statusMap := make(map[string]string)
	for name, ss := range statuses {
		statusMap[name] = ss.Status
	}

	ready2 := dag.ReadySteps(statusMap)
	assert.Equal(t, []string{"c"}, ready2)
}

func TestOrchestrator_DAGFailureCascade(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()
	o := &Orchestrator{
		DB:            database,
		NodeID:        "test-node",
		LeaseDuration: 120 * time.Second,
	}

	execID := createTestExecution(t, database)

	steps := []workflow.Step{
		{Name: "a"},
		{Name: "b"},
		{Name: "c", DependsOn: []string{"a"}},
		{Name: "d", DependsOn: []string{"b"}},
	}

	dag, err := BuildDAG(steps)
	require.NoError(t, err)

	// Create pending steps for initial ready set (a and b).
	ready := dag.ReadySteps(map[string]string{})
	assert.ElementsMatch(t, []string{"a", "b"}, ready)
	err = o.CreatePendingSteps(ctx, execID, ready, map[string]int{})
	require.NoError(t, err)

	// Simulate: a fails, b completes.
	_, err = database.ExecContext(ctx,
		`UPDATE step_executions SET status = 'failed' WHERE execution_id = $1 AND step_name = 'a'`, execID)
	require.NoError(t, err)
	_, err = database.ExecContext(ctx,
		`UPDATE step_executions SET status = 'completed' WHERE execution_id = $1 AND step_name = 'b'`, execID)
	require.NoError(t, err)

	// Get statuses.
	statuses, err := o.GetStepStatuses(ctx, execID)
	require.NoError(t, err)
	statusMap := make(map[string]string)
	for name, ss := range statuses {
		statusMap[name] = ss.Status
	}

	// CascadeCancellations returns []string of steps to cancel.
	cancelled := dag.CascadeCancellations(statusMap)
	assert.Contains(t, cancelled, "c", "c should be cancelled because a failed")
	assert.NotContains(t, cancelled, "d", "d should not be cancelled because b completed")

	// d should be ready (b completed, d depends only on b).
	ready2 := dag.ReadySteps(statusMap)
	assert.Equal(t, []string{"d"}, ready2)

	// Create d as pending and cancel c.
	err = o.CreatePendingSteps(ctx, execID, ready2, map[string]int{})
	require.NoError(t, err)
	err = o.CreatePendingSteps(ctx, execID, []string{"c"}, map[string]int{})
	require.NoError(t, err)
	err = o.CancelPendingSteps(ctx, execID, cancelled)
	require.NoError(t, err)

	// Verify final statuses.
	finalStatuses, err := o.GetStepStatuses(ctx, execID)
	require.NoError(t, err)
	assert.Equal(t, "failed", finalStatuses["a"].Status)
	assert.Equal(t, "completed", finalStatuses["b"].Status)
	assert.Equal(t, "cancelled", finalStatuses["c"].Status)
	assert.Equal(t, "pending", finalStatuses["d"].Status)
}
