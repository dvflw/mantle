package engine

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/dvflw/mantle/internal/db"
	"github.com/dvflw/mantle/internal/workflow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("mantle_test"),
		postgres.WithUsername("mantle"),
		postgres.WithPassword("mantle"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Skipf("Could not start Postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	})
	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("Failed to get connection string: %v", err)
	}

	database, err := db.Open(connStr)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := db.Migrate(ctx, database); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	return database
}

// createTestExecution inserts a workflow_executions row and returns its ID.
func createTestExecution(t *testing.T, database *sql.DB) string {
	t.Helper()
	var id string
	err := database.QueryRow(
		`INSERT INTO workflow_executions (workflow_name, workflow_version, status)
		 VALUES ('test-workflow', 1, 'running')
		 RETURNING id`,
	).Scan(&id)
	if err != nil {
		t.Fatalf("Failed to create test execution: %v", err)
	}
	return id
}

func TestOrchestrator_DAGBasedStepCreation(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()
	o := NewOrchestrator(database)

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
	o := NewOrchestrator(database)

	execID := createTestExecution(t, database)

	// Steps: a, b (independent), c depends on a, d depends on b.
	// a fails -> c should be cancelled, d should still proceed.
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
		`UPDATE step_executions SET status = 'failed' WHERE execution_id = $1 AND step_name = 'a'`,
		execID,
	)
	require.NoError(t, err)
	_, err = database.ExecContext(ctx,
		`UPDATE step_executions SET status = 'completed' WHERE execution_id = $1 AND step_name = 'b'`,
		execID,
	)
	require.NoError(t, err)

	// Get statuses.
	statuses, err := o.GetStepStatuses(ctx, execID)
	require.NoError(t, err)

	statusMap := make(map[string]string)
	for name, ss := range statuses {
		statusMap[name] = ss.Status
	}
	assert.Equal(t, "failed", statusMap["a"])
	assert.Equal(t, "completed", statusMap["b"])

	// Determine cascade cancellations: c should be cancelled (depends on failed a).
	cancelled := dag.CascadeCancellations(statusMap)
	assert.True(t, cancelled["c"], "c should be cancelled because a failed")
	assert.False(t, cancelled["d"], "d should not be cancelled because b completed")

	// Create pending for d (ready because b completed), then cancel c.
	ready2 := dag.ReadySteps(statusMap)
	assert.Equal(t, []string{"d"}, ready2)

	err = o.CreatePendingSteps(ctx, execID, ready2, map[string]int{})
	require.NoError(t, err)

	// Also create c as pending so we can cancel it.
	err = o.CreatePendingSteps(ctx, execID, []string{"c"}, map[string]int{})
	require.NoError(t, err)

	// Cancel the cascaded steps.
	cancelNames := make([]string, 0, len(cancelled))
	for name := range cancelled {
		cancelNames = append(cancelNames, name)
	}
	err = o.CancelPendingSteps(ctx, execID, cancelNames)
	require.NoError(t, err)

	// Verify final statuses: a=failed, b=completed, c=cancelled, d=pending.
	finalStatuses, err := o.GetStepStatuses(ctx, execID)
	require.NoError(t, err)
	assert.Equal(t, "failed", finalStatuses["a"].Status)
	assert.Equal(t, "completed", finalStatuses["b"].Status)
	assert.Equal(t, "cancelled", finalStatuses["c"].Status)
	assert.Equal(t, "pending", finalStatuses["d"].Status)
}
