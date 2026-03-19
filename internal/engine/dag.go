package engine

import (
	"fmt"
	"sort"

	"github.com/dvflw/mantle/internal/workflow"
)

// DAG represents a directed acyclic graph of workflow steps and their dependencies.
type DAG struct {
	// steps maps step name to its definition.
	steps map[string]workflow.Step
	// deps maps step name to the set of step names it depends on.
	deps map[string]map[string]bool
	// dependents maps step name to the set of step names that depend on it.
	dependents map[string]map[string]bool
}

// BuildDAG constructs a DAG from a slice of workflow steps. It validates that
// all dependency references point to existing steps and that no cycles exist.
func BuildDAG(steps []workflow.Step) (*DAG, error) {
	d := &DAG{
		steps:      make(map[string]workflow.Step, len(steps)),
		deps:       make(map[string]map[string]bool, len(steps)),
		dependents: make(map[string]map[string]bool, len(steps)),
	}

	// Index all steps by name.
	for _, s := range steps {
		if _, exists := d.steps[s.Name]; exists {
			return nil, fmt.Errorf("duplicate step name: %s", s.Name)
		}
		d.steps[s.Name] = s
		d.deps[s.Name] = make(map[string]bool)
		d.dependents[s.Name] = make(map[string]bool)
	}

	// Wire up dependency edges.
	for _, s := range steps {
		for _, dep := range s.DependsOn {
			if _, exists := d.steps[dep]; !exists {
				return nil, fmt.Errorf("step %q depends on unknown step %q", s.Name, dep)
			}
			d.deps[s.Name][dep] = true
			d.dependents[dep][s.Name] = true
		}
	}

	// Cycle detection via topological sort (Kahn's algorithm).
	if err := d.detectCycles(); err != nil {
		return nil, err
	}

	return d, nil
}

// ReadySteps returns the names of steps that are ready to execute given the
// current status map (step name -> status string). A step is ready when all its
// dependencies have status "completed" and the step itself has no status yet.
// The returned slice is sorted for deterministic ordering.
func (d *DAG) ReadySteps(statuses map[string]string) []string {
	var ready []string
	for name := range d.steps {
		if _, hasStatus := statuses[name]; hasStatus {
			continue // already started or finished
		}
		allDepsCompleted := true
		for dep := range d.deps[name] {
			if statuses[dep] != "completed" {
				allDepsCompleted = false
				break
			}
		}
		if allDepsCompleted {
			ready = append(ready, name)
		}
	}
	sort.Strings(ready)
	return ready
}

// CascadeCancellations returns the set of step names that should be cancelled
// because one or more of their transitive dependencies have failed. A step is
// cascaded if any of its direct dependencies has status "failed" or is itself
// in the cascade set.
func (d *DAG) CascadeCancellations(statuses map[string]string) map[string]bool {
	cancelled := make(map[string]bool)

	// Use a worklist: start with all directly failed steps' dependents.
	var worklist []string
	for name, status := range statuses {
		if status == "failed" {
			for dep := range d.dependents[name] {
				if !cancelled[dep] {
					cancelled[dep] = true
					worklist = append(worklist, dep)
				}
			}
		}
	}

	// Propagate transitively through dependents.
	for len(worklist) > 0 {
		current := worklist[0]
		worklist = worklist[1:]
		for dep := range d.dependents[current] {
			if !cancelled[dep] {
				cancelled[dep] = true
				worklist = append(worklist, dep)
			}
		}
	}

	return cancelled
}

// detectCycles uses Kahn's algorithm to check for cycles.
func (d *DAG) detectCycles() error {
	inDegree := make(map[string]int, len(d.steps))
	for name := range d.steps {
		inDegree[name] = len(d.deps[name])
	}

	var queue []string
	for name, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, name)
		}
	}

	visited := 0
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		visited++
		for dep := range d.dependents[current] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	if visited != len(d.steps) {
		return fmt.Errorf("cycle detected in step dependencies")
	}
	return nil
}
