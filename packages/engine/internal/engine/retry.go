package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dvflw/mantle/internal/audit"
	"github.com/dvflw/mantle/internal/auth"
	"github.com/dvflw/mantle/internal/workflow"
)

// RetryExecution creates a new execution that resumes from the failure point of
// a previous execution. If fromStep is provided, execution resumes from that
// step; otherwise, the first failed step (in topological order) is used.
// If force is false, concurrency limits are checked before creating the execution;
// if limits are hit the execution is queued instead of started immediately.
func (e *Engine) RetryExecution(ctx context.Context, originalExecID string, fromStep string, force bool) (*ExecutionResult, error) {
	teamID := auth.TeamIDFromContext(ctx)

	// 1. Load original execution metadata.
	var workflowName string
	var version int
	var inputsJSON []byte
	err := e.DB.QueryRowContext(ctx,
		`SELECT workflow_name, workflow_version, inputs
		 FROM workflow_executions
		 WHERE id = $1 AND team_id = $2`,
		originalExecID, teamID,
	).Scan(&workflowName, &version, &inputsJSON)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("execution %q not found", originalExecID)
	}
	if err != nil {
		return nil, fmt.Errorf("loading original execution: %w", err)
	}

	var inputs map[string]any
	if len(inputsJSON) > 0 {
		if err := json.Unmarshal(inputsJSON, &inputs); err != nil {
			return nil, fmt.Errorf("unmarshaling inputs: %w", err)
		}
	}

	// 2. Load the workflow definition (same version as original).
	wf, err := e.loadWorkflow(ctx, workflowName, version)
	if err != nil {
		return nil, fmt.Errorf("loading workflow: %w", err)
	}

	// 3. Determine retry point.
	retryStep := fromStep
	if retryStep == "" {
		statuses, err := loadStepStatuses(ctx, e.DB, originalExecID)
		if err != nil {
			return nil, fmt.Errorf("loading step statuses: %w", err)
		}
		for _, step := range wf.Steps {
			if s, ok := statuses[step.Name]; ok && s == "failed" {
				retryStep = step.Name
				break
			}
		}
		if retryStep == "" {
			return nil, fmt.Errorf("no failed step found in execution %q", originalExecID)
		}
	} else {
		// Validate that the specified step exists.
		found := false
		for _, step := range wf.Steps {
			if step.Name == retryStep {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("step %q not found in workflow %q", retryStep, workflowName)
		}
	}

	// 4. Find all upstream steps (ancestors of the retry point).
	upstream := findUpstream(wf.Steps, retryStep)

	// 5. Check concurrency limits (unless force-bypassed).
	// When limits apply, check + insert happen in the same transaction
	// to prevent a TOCTOU race on the advisory lock.
	var newExecID string
	initialStatus := "pending"
	if !force && (wf.MaxParallelExecutions > 0 || e.MaxConcurrentExecutionsPerTeam > 0) {
		tx, txErr := e.DB.BeginTx(ctx, nil)
		if txErr != nil {
			return nil, fmt.Errorf("starting concurrency check transaction: %w", txErr)
		}
		defer tx.Rollback()

		cr := CheckConcurrencyLimits(ctx, tx, workflowName,
			wf.MaxParallelExecutions, wf.OnLimit, e.MaxConcurrentExecutionsPerTeam)

		if cr.Err != nil {
			return nil, cr.Err
		}
		if cr.Queued {
			initialStatus = "queued"
		}

		// Insert execution row inside the same transaction.
		newExecID, err = e.createExecutionTx(ctx, tx, workflowName, version, inputs, initialStatus)
		if err != nil {
			return nil, fmt.Errorf("creating retry execution: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("committing concurrency check: %w", err)
		}
	} else {
		// No concurrency limits — use implicit transaction.
		newExecID, err = e.createExecution(ctx, workflowName, version, inputs, initialStatus)
		if err != nil {
			return nil, fmt.Errorf("creating retry execution: %w", err)
		}
	}

	// 7. Link new execution to original.
	_, err = e.DB.ExecContext(ctx,
		`UPDATE workflow_executions SET retried_from_execution_id = $1 WHERE id = $2 AND team_id = $3`,
		originalExecID, newExecID, teamID,
	)
	if err != nil {
		return nil, fmt.Errorf("linking retry execution: %w", err)
	}

	// 8. Copy completed upstream step outputs from original (exclude hook steps).
	upstreamSet := make(map[string]bool, len(upstream))
	for _, name := range upstream {
		upstreamSet[name] = true
	}

	rows, err := e.DB.QueryContext(ctx,
		`SELECT DISTINCT ON (step_name) step_name, status, output, error
		 FROM step_executions
		 WHERE execution_id = $1 AND hook_block IS NULL
		 ORDER BY step_name, attempt DESC`,
		originalExecID,
	)
	if err != nil {
		return nil, fmt.Errorf("loading original steps: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var stepName, status string
		var outputJSON []byte
		var stepErr *string
		if err := rows.Scan(&stepName, &status, &outputJSON, &stepErr); err != nil {
			return nil, fmt.Errorf("scanning step: %w", err)
		}
		// Only copy completed upstream steps.
		if !upstreamSet[stepName] || status != "completed" {
			continue
		}
		errVal := ""
		if stepErr != nil {
			errVal = *stepErr
		}
		var errPtr *string
		if errVal != "" {
			errPtr = &errVal
		}
		_, err := e.DB.ExecContext(ctx,
			`INSERT INTO step_executions (execution_id, step_name, attempt, status, output, error, started_at)
			 VALUES ($1, $2, 1, $3, $4, $5, NOW())
			 ON CONFLICT (execution_id, step_name, attempt) WHERE hook_block IS NULL DO NOTHING`,
			newExecID, stepName, status, outputJSON, errPtr,
		)
		if err != nil {
			return nil, fmt.Errorf("copying step %q: %w", stepName, err)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating original steps: %w", err)
	}

	// 9. Emit audit event.
	e.Auditor.Emit(ctx, audit.Event{
		Timestamp: time.Now(),
		Actor:     "engine",
		Action:    audit.ActionExecutionRetried,
		Resource:  audit.Resource{Type: "workflow_execution", ID: newExecID},
		Metadata: map[string]string{
			"original_execution_id": originalExecID,
			"from_step":             retryStep,
			"workflow":              workflowName,
			"version":               fmt.Sprintf("%d", version),
		},
	})

	// 10. If queued, return immediately with queued status.
	if initialStatus == "queued" {
		return &ExecutionResult{
			ExecutionID: newExecID,
			Status:      "queued",
		}, nil
	}

	// 11. Run the new execution (resumeExecution will skip already-completed steps).
	return e.resumeExecution(ctx, newExecID, workflowName, version, inputs)
}

// findUpstream returns the names of all steps that are strictly upstream
// (ancestors) of the target step in the dependency graph.
func findUpstream(steps []workflow.Step, target string) []string {
	// Build dependency map.
	deps := make(map[string][]string, len(steps))
	for _, s := range steps {
		deps[s.Name] = s.DependsOn
	}

	// Walk backwards from target, collecting all ancestors.
	visited := make(map[string]bool)
	var walk func(name string)
	walk = func(name string) {
		for _, dep := range deps[name] {
			if !visited[dep] {
				visited[dep] = true
				walk(dep)
			}
		}
	}
	walk(target)

	result := make([]string, 0, len(visited))
	for name := range visited {
		result = append(result, name)
	}
	return result
}

// loadStepStatuses loads the latest status per main step (excluding hooks)
// for a given execution.
func loadStepStatuses(ctx context.Context, db *sql.DB, execID string) (map[string]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT DISTINCT ON (step_name) step_name, status
		 FROM step_executions
		 WHERE execution_id = $1 AND hook_block IS NULL
		 ORDER BY step_name, attempt DESC`,
		execID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var name, status string
		if err := rows.Scan(&name, &status); err != nil {
			return nil, err
		}
		result[name] = status
	}
	return result, rows.Err()
}
