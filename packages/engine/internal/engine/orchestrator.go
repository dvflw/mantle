package engine

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

// StepStatus represents the current status of a step execution.
type StepStatus struct {
	Status      string
	Attempt     int
	MaxAttempts int
	Output      map[string]any
	Error       string
}

// Orchestrator coordinates multi-node execution claims and step lifecycle
// using Postgres advisory locks and SKIP LOCKED for distributed coordination.
type Orchestrator struct {
	DB            *sql.DB
	NodeID        string
	LeaseDuration time.Duration
	PollInterval  time.Duration
	Logger        *slog.Logger
}

// ClaimExecution attempts to claim an execution for this node.
// Returns true if the claim was acquired, false if another node already holds it.
// Uses INSERT ... ON CONFLICT DO NOTHING for atomic claim acquisition.
func (o *Orchestrator) ClaimExecution(ctx context.Context, executionID string) (bool, error) {
	interval := fmt.Sprintf("%d", int(o.LeaseDuration.Seconds()))
	result, err := o.DB.ExecContext(ctx,
		`INSERT INTO execution_claims (execution_id, claimed_by, lease_expires_at)
		 VALUES ($1, $2, NOW() + ($3 || ' seconds')::interval)
		 ON CONFLICT (execution_id) DO NOTHING`,
		executionID, o.NodeID, interval,
	)
	if err != nil {
		return false, fmt.Errorf("claiming execution %s: %w", executionID, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("checking claim result for %s: %w", executionID, err)
	}
	return rows == 1, nil
}

// RenewExecutionLease extends the lease expiry for an execution claimed by this node.
// The update is fenced by claimed_by to prevent a node from renewing another node's lease.
func (o *Orchestrator) RenewExecutionLease(ctx context.Context, executionID string) error {
	interval := fmt.Sprintf("%d", int(o.LeaseDuration.Seconds()))
	result, err := o.DB.ExecContext(ctx,
		`UPDATE execution_claims
		 SET lease_expires_at = NOW() + ($2 || ' seconds')::interval
		 WHERE execution_id = $1 AND claimed_by = $3`,
		executionID, interval, o.NodeID,
	)
	if err != nil {
		return fmt.Errorf("renewing lease for execution %s: %w", executionID, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking renew result for %s: %w", executionID, err)
	}
	if rows == 0 {
		return fmt.Errorf("lease renewal failed for execution %s: not claimed by this node", executionID)
	}
	return nil
}

// ReleaseExecution releases the claim on an execution held by this node.
// The delete is fenced by claimed_by to prevent releasing another node's claim.
func (o *Orchestrator) ReleaseExecution(ctx context.Context, executionID string) error {
	result, err := o.DB.ExecContext(ctx,
		`DELETE FROM execution_claims
		 WHERE execution_id = $1 AND claimed_by = $2`,
		executionID, o.NodeID,
	)
	if err != nil {
		return fmt.Errorf("releasing execution %s: %w", executionID, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking release result for %s: %w", executionID, err)
	}
	if rows == 0 {
		return fmt.Errorf("release failed for execution %s: not claimed by this node", executionID)
	}
	return nil
}

// CreatePendingSteps inserts pending step_execution rows for the given step names.
// Uses ON CONFLICT DO NOTHING for idempotent creation (safe to call multiple times).
func (o *Orchestrator) CreatePendingSteps(ctx context.Context, executionID string, stepNames []string, maxAttempts map[string]int) error {
	tx, err := o.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO step_executions (execution_id, step_name, attempt, status, max_attempts)
		 VALUES ($1, $2, 1, 'pending', $3)
		 ON CONFLICT (execution_id, step_name, attempt) WHERE hook_block IS NULL DO NOTHING`,
	)
	if err != nil {
		return fmt.Errorf("preparing statement: %w", err)
	}
	defer stmt.Close()

	for _, name := range stepNames {
		ma := 1
		if v, ok := maxAttempts[name]; ok {
			ma = v
		}
		if _, err := stmt.ExecContext(ctx, executionID, name, ma); err != nil {
			return fmt.Errorf("inserting step %s: %w", name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing pending steps: %w", err)
	}
	return nil
}

// CreateRetryStep creates a new attempt row for a step that needs to be retried.
func (o *Orchestrator) CreateRetryStep(ctx context.Context, executionID, stepName string, nextAttempt, maxAttempts int) error {
	_, err := o.DB.ExecContext(ctx,
		`INSERT INTO step_executions (execution_id, step_name, attempt, status, max_attempts)
		 VALUES ($1, $2, $3, 'pending', $4)
		 ON CONFLICT (execution_id, step_name, attempt) WHERE hook_block IS NULL DO NOTHING`,
		executionID, stepName, nextAttempt, maxAttempts,
	)
	if err != nil {
		return fmt.Errorf("creating retry step %s attempt %d: %w", stepName, nextAttempt, err)
	}
	return nil
}

// GetStepStatuses returns the latest status for each top-level step in an execution.
// Uses DISTINCT ON to return only the most recent attempt per step name.
func (o *Orchestrator) GetStepStatuses(ctx context.Context, executionID string) (map[string]StepStatus, error) {
	rows, err := o.DB.QueryContext(ctx,
		`SELECT DISTINCT ON (step_name) step_name, status, attempt, max_attempts, output, error
		 FROM step_executions
		 WHERE execution_id = $1 AND parent_step_id IS NULL AND hook_block IS NULL
		 ORDER BY step_name, attempt DESC`,
		executionID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying step statuses for %s: %w", executionID, err)
	}
	defer rows.Close()

	statuses := make(map[string]StepStatus)
	for rows.Next() {
		var (
			name       string
			s          StepStatus
			outputJSON sql.NullString
			errStr     sql.NullString
		)
		if err := rows.Scan(&name, &s.Status, &s.Attempt, &s.MaxAttempts, &outputJSON, &errStr); err != nil {
			return nil, fmt.Errorf("scanning step status: %w", err)
		}
		if errStr.Valid {
			s.Error = errStr.String
		}
		// Output parsing is left to callers that need it; we store the raw status here.
		statuses[name] = s
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating step statuses: %w", err)
	}
	return statuses, nil
}

// AdvanceExecution examines the current state of an execution and starts any
// steps whose dependencies are satisfied.
//
// Steps with continue_on_error=true that have failed are treated as resolved
// (equivalent to completed) for dependency resolution purposes, so their
// downstream steps can still be scheduled. They do NOT propagate cancellations.
//
// CAVEAT: continue_on_error is resolved from the in-memory steps parameter,
// not from the database. If a workflow definition is re-applied via
// "mantle apply" while an execution is in flight, the orchestrator may use
// a stale continue_on_error value for already-started executions. This is
// by design — mid-execution definition changes are not guaranteed to take
// effect until the next execution starts.
//
// Returns the names of newly-created pending steps, or nil if none were ready.
func (o *Orchestrator) AdvanceExecution(ctx context.Context, executionID string, steps []workflowStep) ([]string, error) {
	statuses, err := o.GetStepStatuses(ctx, executionID)
	if err != nil {
		return nil, fmt.Errorf("getting step statuses: %w", err)
	}

	// Build a status map for DAG queries, remapping failed-but-continued steps
	// to "completed" so the DAG treats them as resolved.
	continuedNames := make(map[string]bool)
	for _, s := range steps {
		if s.ContinueOnError {
			continuedNames[s.Name] = true
		}
	}

	dagStatuses := make(map[string]string, len(statuses))
	for name, ss := range statuses {
		st := ss.Status
		if st == "failed" && continuedNames[name] {
			st = "completed"
		}
		dagStatuses[name] = st
	}

	// Convert workflowStep slice to workflow.Step slice for DAG construction.
	wfSteps := make([]workflowStepForDAG, len(steps))
	for i, s := range steps {
		wfSteps[i] = workflowStepForDAG{Name: s.Name, DependsOn: s.DependsOn}
	}
	dag, err := buildDAGFromSteps(wfSteps)
	if err != nil {
		return nil, fmt.Errorf("building DAG: %w", err)
	}

	// Cascade cancellations for non-continued failures.
	toCancel := dag.CascadeCancellations(dagStatuses)
	if len(toCancel) > 0 {
		if err := o.CancelPendingSteps(ctx, executionID, toCancel); err != nil {
			return nil, fmt.Errorf("cascading cancellations: %w", err)
		}
		for _, name := range toCancel {
			dagStatuses[name] = "cancelled"
		}
	}

	// Determine which steps are now ready.
	ready := dag.ReadySteps(dagStatuses)
	if len(ready) == 0 {
		return nil, nil
	}

	// Build maxAttempts map.
	maxAttempts := make(map[string]int, len(steps))
	for _, s := range steps {
		if s.MaxAttempts > 0 {
			maxAttempts[s.Name] = s.MaxAttempts
		}
	}

	if err := o.CreatePendingSteps(ctx, executionID, ready, maxAttempts); err != nil {
		return nil, fmt.Errorf("creating pending steps: %w", err)
	}
	return ready, nil
}

// workflowStep is a minimal step descriptor used by AdvanceExecution to avoid
// importing the workflow package directly in orchestrator logic.
type workflowStep struct {
	Name            string
	DependsOn       []string
	ContinueOnError bool
	MaxAttempts     int
}

// workflowStepForDAG is used internally to build a DAG from minimal step info.
type workflowStepForDAG struct {
	Name      string
	DependsOn []string
}

// buildDAGFromSteps builds a DAG from the minimal step descriptors.
func buildDAGFromSteps(steps []workflowStepForDAG) (*dagForAdvance, error) {
	d := &dagForAdvance{
		deps:  make(map[string][]string, len(steps)),
		rdeps: make(map[string][]string, len(steps)),
	}
	names := make(map[string]bool, len(steps))
	for _, s := range steps {
		names[s.Name] = true
		d.deps[s.Name] = nil
		d.rdeps[s.Name] = nil
	}
	for _, s := range steps {
		for _, dep := range s.DependsOn {
			if !names[dep] {
				return nil, fmt.Errorf("step %q depends on undefined step %q", s.Name, dep)
			}
			d.deps[s.Name] = append(d.deps[s.Name], dep)
			d.rdeps[dep] = append(d.rdeps[dep], s.Name)
		}
	}
	order, err := topoSortSimple(d.deps, d.rdeps, names)
	if err != nil {
		return nil, err
	}
	d.order = order
	return d, nil
}

// dagForAdvance is a lightweight DAG used by AdvanceExecution.
type dagForAdvance struct {
	deps  map[string][]string
	rdeps map[string][]string
	order []string
}

// ReadySteps returns step names whose dependencies are all resolved
// (completed or skipped) and that do not yet have a status entry.
func (d *dagForAdvance) ReadySteps(statuses map[string]string) []string {
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
// dependency failed (and was not remapped to completed by continue_on_error).
func (d *dagForAdvance) CascadeCancellations(statuses map[string]string) []string {
	poisoned := make(map[string]bool)
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

// topoSortSimple performs Kahn's topological sort over a name-keyed dep graph.
func topoSortSimple(deps, rdeps map[string][]string, names map[string]bool) ([]string, error) {
	inDegree := make(map[string]int, len(names))
	for name := range names {
		inDegree[name] = len(deps[name])
	}
	var queue []string
	for name, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, name)
		}
	}
	// Sort for deterministic ordering.
	sortStrings(queue)

	var order []string
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		order = append(order, node)
		for _, dependent := range rdeps[node] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
				sortStrings(queue)
			}
		}
	}
	if len(order) != len(names) {
		return nil, fmt.Errorf("cycle detected in step dependencies")
	}
	return order, nil
}

// sortStrings sorts a slice of strings in-place (insertion sort for small slices).
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// CancelPendingSteps sets all pending steps with the given names to cancelled status.
func (o *Orchestrator) CancelPendingSteps(ctx context.Context, executionID string, stepNames []string) error {
	if len(stepNames) == 0 {
		return nil
	}

	tx, err := o.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`UPDATE step_executions
		 SET status = 'cancelled', updated_at = NOW()
		 WHERE execution_id = $1 AND status = 'pending' AND step_name = $2`,
	)
	if err != nil {
		return fmt.Errorf("preparing cancel statement: %w", err)
	}
	defer stmt.Close()

	for _, name := range stepNames {
		if _, err := stmt.ExecContext(ctx, executionID, name); err != nil {
			return fmt.Errorf("cancelling step %s: %w", name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing cancel: %w", err)
	}
	return nil
}
