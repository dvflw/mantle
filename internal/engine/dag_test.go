package engine

import (
	"slices"
	"strings"
	"testing"

	"github.com/dvflw/mantle/internal/workflow"
)

func TestDAG_LinearChain(t *testing.T) {
	steps := []workflow.Step{
		{Name: "a"},
		{Name: "b", DependsOn: []string{"a"}},
		{Name: "c", DependsOn: []string{"b"}},
	}

	dag, err := BuildDAG(steps)
	if err != nil {
		t.Fatalf("BuildDAG: %v", err)
	}

	// Initially only "a" is ready.
	ready := dag.ReadySteps(map[string]string{})
	assertNames(t, ready, []string{"a"}, "initial ready")

	// After a completes, b is ready.
	ready = dag.ReadySteps(map[string]string{"a": "completed"})
	assertNames(t, ready, []string{"b"}, "after a completed")

	// After a and b complete, c is ready.
	ready = dag.ReadySteps(map[string]string{"a": "completed", "b": "completed"})
	assertNames(t, ready, []string{"c"}, "after a,b completed")

	// After all complete, nothing ready.
	ready = dag.ReadySteps(map[string]string{"a": "completed", "b": "completed", "c": "completed"})
	assertNames(t, ready, nil, "all completed")
}

func TestDAG_ParallelSteps(t *testing.T) {
	steps := []workflow.Step{
		{Name: "a"},
		{Name: "b"},
		{Name: "c", DependsOn: []string{"a", "b"}},
	}

	dag, err := BuildDAG(steps)
	if err != nil {
		t.Fatalf("BuildDAG: %v", err)
	}

	// a and b should both be ready initially.
	ready := dag.ReadySteps(map[string]string{})
	assertNames(t, ready, []string{"a", "b"}, "initial ready")

	// Only a completed -- c still blocked on b.
	ready = dag.ReadySteps(map[string]string{"a": "completed"})
	assertNames(t, ready, []string{"b"}, "after a completed")

	// Both completed -- c now ready.
	ready = dag.ReadySteps(map[string]string{"a": "completed", "b": "completed"})
	assertNames(t, ready, []string{"c"}, "after a,b completed")
}

func TestDAG_SkippedDependency(t *testing.T) {
	steps := []workflow.Step{
		{Name: "a"},
		{Name: "b", DependsOn: []string{"a"}},
	}

	dag, err := BuildDAG(steps)
	if err != nil {
		t.Fatalf("BuildDAG: %v", err)
	}

	// Skipped counts as resolved.
	ready := dag.ReadySteps(map[string]string{"a": "skipped"})
	assertNames(t, ready, []string{"b"}, "after a skipped")
}

func TestDAG_CycleDetection(t *testing.T) {
	steps := []workflow.Step{
		{Name: "a", DependsOn: []string{"c"}},
		{Name: "b", DependsOn: []string{"a"}},
		{Name: "c", DependsOn: []string{"b"}},
	}

	_, err := BuildDAG(steps)
	if err == nil {
		t.Fatal("expected error for cycle, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle error, got: %v", err)
	}
}

func TestDAG_FailureCascade(t *testing.T) {
	// a and b are independent roots.
	// c depends on a (will be poisoned).
	// d depends on b (independent, unaffected).
	// e depends on c (transitively poisoned).
	steps := []workflow.Step{
		{Name: "a"},
		{Name: "b"},
		{Name: "c", DependsOn: []string{"a"}},
		{Name: "d", DependsOn: []string{"b"}},
		{Name: "e", DependsOn: []string{"c"}},
	}

	dag, err := BuildDAG(steps)
	if err != nil {
		t.Fatalf("BuildDAG: %v", err)
	}

	statuses := map[string]string{
		"a": "failed",
		"b": "completed",
	}

	cancelled := dag.CascadeCancellations(statuses)
	assertNames(t, cancelled, []string{"c", "e"}, "cascade from a failed")
}

func TestDAG_UndefinedDependency(t *testing.T) {
	steps := []workflow.Step{
		{Name: "a", DependsOn: []string{"nonexistent"}},
	}

	_, err := BuildDAG(steps)
	if err == nil {
		t.Fatal("expected error for undefined dependency, got nil")
	}
	if !strings.Contains(err.Error(), "undefined") {
		t.Fatalf("expected undefined step error, got: %v", err)
	}
}

func TestDAG_AddImplicitDeps(t *testing.T) {
	steps := []workflow.Step{
		{Name: "a"},
		{Name: "b"},
		{Name: "c"},
	}

	dag, err := BuildDAG(steps)
	if err != nil {
		t.Fatalf("BuildDAG: %v", err)
	}

	// All three should be ready initially.
	ready := dag.ReadySteps(map[string]string{})
	assertNames(t, ready, []string{"a", "b", "c"}, "initial ready")

	// Add implicit dep: c depends on a.
	err = dag.AddImplicitDeps(map[string][]string{"c": {"a"}})
	if err != nil {
		t.Fatalf("AddImplicitDeps: %v", err)
	}

	ready = dag.ReadySteps(map[string]string{})
	assertNames(t, ready, []string{"a", "b"}, "after implicit dep added")

	// Adding a cycle should fail.
	err = dag.AddImplicitDeps(map[string][]string{"a": {"c"}})
	if err == nil {
		t.Fatal("expected cycle error after implicit dep, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle error, got: %v", err)
	}
}

// assertNames checks that got contains exactly the expected names (order-independent).
func assertNames(t *testing.T, got, want []string, context string) {
	t.Helper()
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("[%s] got %v, want %v", context, got, want)
	}
}
