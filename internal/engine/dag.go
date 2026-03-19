package engine

import (
	"fmt"
	"slices"

	"github.com/dvflw/mantle/internal/workflow"
)

// DAG represents a directed acyclic graph of workflow steps, used to determine
// execution order and parallelism opportunities.
type DAG struct {
	steps map[string]*workflow.Step
	deps  map[string][]string // step -> dependencies
	rdeps map[string][]string // step -> reverse dependencies (dependents)
	order []string            // topological order
}

// BuildDAG constructs a DAG from a slice of workflow steps. It validates that
// all dependency references exist and that no cycles are present. The
// topological order is computed using Kahn's algorithm.
func BuildDAG(steps []workflow.Step) (*DAG, error) {
	d := &DAG{
		steps: make(map[string]*workflow.Step, len(steps)),
		deps:  make(map[string][]string, len(steps)),
		rdeps: make(map[string][]string, len(steps)),
	}

	// Index steps by name.
	for i := range steps {
		s := &steps[i]
		if _, exists := d.steps[s.Name]; exists {
			return nil, fmt.Errorf("duplicate step name: %s", s.Name)
		}
		d.steps[s.Name] = s
		d.deps[s.Name] = nil
		d.rdeps[s.Name] = nil
	}

	// Wire up dependency and reverse-dependency edges.
	for i := range steps {
		s := &steps[i]
		for _, dep := range s.DependsOn {
			if _, exists := d.steps[dep]; !exists {
				return nil, fmt.Errorf("step %q depends on undefined step %q", s.Name, dep)
			}
			d.deps[s.Name] = append(d.deps[s.Name], dep)
			d.rdeps[dep] = append(d.rdeps[dep], s.Name)
		}
	}

	order, err := topoSort(d.deps, d.steps)
	if err != nil {
		return nil, err
	}
	d.order = order

	return d, nil
}

// ReadySteps returns step names whose dependencies are all resolved. A
// dependency is resolved when its status is "completed" or "skipped". Steps
// that already have a status entry are excluded from the result.
func (d *DAG) ReadySteps(statuses map[string]string) []string {
	var ready []string
	for _, name := range d.order {
		if _, has := statuses[name]; has {
			continue
		}
		allResolved := true
		for _, dep := range d.deps[name] {
			st, ok := statuses[dep]
			if !ok || (st != "completed" && st != "skipped") {
				allResolved = false
				break
			}
		}
		if allResolved {
			ready = append(ready, name)
		}
	}
	return ready
}

// CascadeCancellations returns step names that should be cancelled because a
// dependency transitively failed. It walks forward through topological order:
// if any dependency of a step is "failed" or already poisoned, that step is
// poisoned too. Only steps not already present in statuses are returned.
func (d *DAG) CascadeCancellations(statuses map[string]string) []string {
	poisoned := make(map[string]bool)

	// Seed poisoned set from statuses.
	for name, st := range statuses {
		if st == "failed" {
			poisoned[name] = true
		}
	}

	var cancelled []string
	for _, name := range d.order {
		if poisoned[name] {
			continue
		}
		for _, dep := range d.deps[name] {
			if poisoned[dep] {
				poisoned[name] = true
				break
			}
		}
		if poisoned[name] {
			if _, has := statuses[name]; !has {
				cancelled = append(cancelled, name)
			}
		}
	}
	return cancelled
}

// AddImplicitDeps merges additional dependencies (e.g. from CEL expression
// analysis) into the DAG. It deduplicates against existing deps and
// re-validates for cycles after adding.
func (d *DAG) AddImplicitDeps(implicit map[string][]string) error {
	for step, newDeps := range implicit {
		if _, exists := d.steps[step]; !exists {
			return fmt.Errorf("implicit dep target %q is not a known step", step)
		}
		for _, dep := range newDeps {
			if _, exists := d.steps[dep]; !exists {
				return fmt.Errorf("implicit dep %q for step %q is not a known step", dep, step)
			}
			if !slices.Contains(d.deps[step], dep) {
				d.deps[step] = append(d.deps[step], dep)
				d.rdeps[dep] = append(d.rdeps[dep], step)
			}
		}
	}

	order, err := topoSort(d.deps, d.steps)
	if err != nil {
		return err
	}
	d.order = order
	return nil
}

// Order returns a copy of the topological order for inspection.
func (d *DAG) Order() []string {
	return slices.Clone(d.order)
}

// topoSort performs a topological sort using Kahn's algorithm. It returns an
// error if a cycle is detected.
func topoSort(deps map[string][]string, steps map[string]*workflow.Step) ([]string, error) {
	inDegree := make(map[string]int, len(steps))
	for name := range steps {
		inDegree[name] = len(deps[name])
	}

	// Collect nodes with zero in-degree.
	var queue []string
	for name, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, name)
		}
	}
	slices.Sort(queue) // deterministic ordering

	var order []string
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		order = append(order, node)

		// For each step that depends on this node, decrement in-degree.
		for other, otherDeps := range deps {
			for _, d := range otherDeps {
				if d == node {
					inDegree[other]--
					if inDegree[other] == 0 {
						// Insert sorted to keep deterministic order.
						idx, _ := slices.BinarySearch(queue, other)
						queue = slices.Insert(queue, idx, other)
					}
					break
				}
			}
		}
	}

	if len(order) != len(steps) {
		return nil, fmt.Errorf("cycle detected in step dependencies")
	}
	return order, nil
}
