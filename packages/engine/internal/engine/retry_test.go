package engine

import (
	"context"
	"sort"
	"testing"

	"github.com/dvflw/mantle/internal/workflow"
)

func TestFindUpstream_LinearChain(t *testing.T) {
	// A → B → C
	steps := []workflow.Step{
		{Name: "A"},
		{Name: "B", DependsOn: []string{"A"}},
		{Name: "C", DependsOn: []string{"B"}},
	}

	upstream := findUpstream(steps, "C")
	sort.Strings(upstream)

	expected := []string{"A", "B"}
	if len(upstream) != len(expected) {
		t.Fatalf("findUpstream(C) = %v, want %v", upstream, expected)
	}
	for i, name := range expected {
		if upstream[i] != name {
			t.Errorf("upstream[%d] = %q, want %q", i, upstream[i], name)
		}
	}
}

func TestFindUpstream_Diamond(t *testing.T) {
	// A → B, A → C, B → D, C → D
	steps := []workflow.Step{
		{Name: "A"},
		{Name: "B", DependsOn: []string{"A"}},
		{Name: "C", DependsOn: []string{"A"}},
		{Name: "D", DependsOn: []string{"B", "C"}},
	}

	upstream := findUpstream(steps, "D")
	sort.Strings(upstream)

	expected := []string{"A", "B", "C"}
	if len(upstream) != len(expected) {
		t.Fatalf("findUpstream(D) = %v, want %v", upstream, expected)
	}
	for i, name := range expected {
		if upstream[i] != name {
			t.Errorf("upstream[%d] = %q, want %q", i, upstream[i], name)
		}
	}
}

func TestFindUpstream_NoUpstream(t *testing.T) {
	steps := []workflow.Step{
		{Name: "root"},
		{Name: "child", DependsOn: []string{"root"}},
	}

	upstream := findUpstream(steps, "root")
	if len(upstream) != 0 {
		t.Errorf("findUpstream(root) = %v, want empty slice", upstream)
	}
}

func TestLoadStepStatuses_ExcludesHookSteps(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	execID := createTestExecution(t, database)

	// Insert a main step (hook_block IS NULL).
	_, err := database.ExecContext(ctx,
		`INSERT INTO step_executions (execution_id, step_name, attempt, status, started_at)
		 VALUES ($1, 'main-step', 1, 'completed', NOW())`,
		execID)
	if err != nil {
		t.Fatalf("inserting main step: %v", err)
	}

	// Insert a hook step (hook_block = 'on_success').
	_, err = database.ExecContext(ctx,
		`INSERT INTO step_executions (execution_id, step_name, attempt, status, hook_block, started_at)
		 VALUES ($1, 'hook-step', 1, 'completed', 'on_success', NOW())`,
		execID)
	if err != nil {
		t.Fatalf("inserting hook step: %v", err)
	}

	statuses, err := loadStepStatuses(ctx, database, execID)
	if err != nil {
		t.Fatalf("loadStepStatuses() error: %v", err)
	}

	// Should contain main-step but not hook-step.
	if _, ok := statuses["main-step"]; !ok {
		t.Error("expected main-step in statuses, not found")
	}
	if statuses["main-step"] != "completed" {
		t.Errorf("main-step status = %q, want %q", statuses["main-step"], "completed")
	}
	if _, ok := statuses["hook-step"]; ok {
		t.Error("hook-step should be excluded from statuses but was found")
	}
}
