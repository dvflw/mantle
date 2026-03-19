package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dvflw/mantle/internal/audit"
	mantleCEL "github.com/dvflw/mantle/internal/cel"
	"github.com/dvflw/mantle/internal/connector"
	"github.com/dvflw/mantle/internal/secret"
	"github.com/dvflw/mantle/internal/workflow"
)

// Engine executes workflows by running steps sequentially with checkpoint-and-resume.
type Engine struct {
	DB       *sql.DB
	Registry *connector.Registry
	Auditor  audit.Emitter
	CEL      *mantleCEL.Evaluator
	Resolver *secret.Resolver
}

// New creates an Engine with sensible defaults.
func New(db *sql.DB) (*Engine, error) {
	celEval, err := mantleCEL.NewEvaluator()
	if err != nil {
		return nil, fmt.Errorf("creating CEL evaluator: %w", err)
	}
	return &Engine{
		DB:       db,
		Registry: connector.NewRegistry(),
		Auditor:  &audit.NoopEmitter{},
		CEL:      celEval,
		Resolver: &secret.Resolver{Store: nil},
	}, nil
}

// ExecutionResult holds the outcome of a workflow execution.
type ExecutionResult struct {
	ExecutionID string
	Status      string // "completed" or "failed"
	Error       string
	Steps       map[string]StepResult
}

// StepResult holds the outcome of a single step.
type StepResult struct {
	Status string         // "completed", "failed", "skipped"
	Output map[string]any // step output data
	Error  string
}

// Execute runs a workflow by name, pinned to the specified version.
func (e *Engine) Execute(ctx context.Context, workflowName string, version int, inputs map[string]any) (*ExecutionResult, error) {
	// Create execution record.
	execID, err := e.createExecution(ctx, workflowName, version, inputs)
	if err != nil {
		return nil, fmt.Errorf("creating execution: %w", err)
	}

	return e.resumeExecution(ctx, execID, workflowName, version, inputs)
}

// resumeExecution runs (or resumes) a workflow execution from its last checkpoint.
func (e *Engine) resumeExecution(ctx context.Context, execID string, workflowName string, version int, inputs map[string]any) (*ExecutionResult, error) {
	// Load the workflow definition.
	wf, err := e.loadWorkflow(ctx, workflowName, version)
	if err != nil {
		return nil, fmt.Errorf("loading workflow: %w", err)
	}

	e.Auditor.Emit(ctx, audit.Event{
		Timestamp: time.Now(),
		Actor:     "engine",
		Action:    audit.ActionWorkflowExecuted,
		Resource:  audit.Resource{Type: "workflow_execution", ID: execID},
		Metadata:  map[string]string{"workflow": workflowName, "version": fmt.Sprintf("%d", version)},
	})

	// Build CEL context with inputs.
	celCtx := &mantleCEL.Context{
		Steps:  make(map[string]map[string]any),
		Inputs: inputs,
	}
	if celCtx.Inputs == nil {
		celCtx.Inputs = make(map[string]any)
	}

	// Load already-completed steps for checkpoint recovery.
	completedSteps, err := e.loadCompletedSteps(ctx, execID)
	if err != nil {
		return nil, fmt.Errorf("loading completed steps: %w", err)
	}
	for name, output := range completedSteps {
		celCtx.Steps[name] = map[string]any{"output": output}
	}

	result := &ExecutionResult{
		ExecutionID: execID,
		Status:      "completed",
		Steps:       make(map[string]StepResult),
	}

	// Update execution status to running.
	if err := e.updateExecutionStatus(ctx, execID, "running", ""); err != nil {
		return nil, fmt.Errorf("checkpoint: %w", err)
	}

	// Execute steps sequentially.
	for _, step := range wf.Steps {
		// Skip already-completed steps (checkpoint recovery).
		if _, done := completedSteps[step.Name]; done {
			result.Steps[step.Name] = StepResult{
				Status: "completed",
				Output: completedSteps[step.Name],
			}
			continue
		}

		stepResult := e.executeStep(ctx, execID, step, celCtx)
		result.Steps[step.Name] = stepResult

		if stepResult.Status == "completed" {
			celCtx.Steps[step.Name] = map[string]any{"output": stepResult.Output}
		} else if stepResult.Status == "skipped" {
			celCtx.Steps[step.Name] = map[string]any{"output": map[string]any{}}
		} else if stepResult.Status == "failed" {
			result.Status = "failed"
			result.Error = fmt.Sprintf("step %q failed: %s", step.Name, stepResult.Error)
			if err := e.updateExecutionStatus(ctx, execID, "failed", result.Error); err != nil {
				return nil, fmt.Errorf("checkpoint: %w", err)
			}
			return result, nil
		}
	}

	if err := e.updateExecutionStatus(ctx, execID, "completed", ""); err != nil {
		return nil, fmt.Errorf("checkpoint: %w", err)
	}
	return result, nil
}

func (e *Engine) executeStep(ctx context.Context, execID string, step workflow.Step, celCtx *mantleCEL.Context) StepResult {
	// Evaluate conditional.
	if step.If != "" {
		shouldRun, err := e.CEL.EvalBool(step.If, celCtx)
		if err != nil {
			if cpErr := e.recordStep(ctx, execID, step.Name, "failed", nil, err.Error()); cpErr != nil {
				return StepResult{Status: "failed", Error: fmt.Sprintf("checkpoint failure: %v (original: evaluating if condition: %v)", cpErr, err)}
			}
			return StepResult{Status: "failed", Error: fmt.Sprintf("evaluating if condition: %v", err)}
		}
		if !shouldRun {
			if cpErr := e.recordStep(ctx, execID, step.Name, "skipped", nil, ""); cpErr != nil {
				return StepResult{Status: "failed", Error: fmt.Sprintf("checkpoint failure: %v", cpErr)}
			}
			e.Auditor.Emit(ctx, audit.Event{
				Timestamp: time.Now(),
				Actor:     "engine",
				Action:    audit.ActionStepSkipped,
				Resource:  audit.Resource{Type: "step_execution", ID: step.Name},
			})
			return StepResult{Status: "skipped"}
		}
	}

	// Record step as running.
	if cpErr := e.recordStep(ctx, execID, step.Name, "running", nil, ""); cpErr != nil {
		return StepResult{Status: "failed", Error: fmt.Sprintf("checkpoint failure: %v", cpErr)}
	}
	e.Auditor.Emit(ctx, audit.Event{
		Timestamp: time.Now(),
		Actor:     "engine",
		Action:    audit.ActionStepStarted,
		Resource:  audit.Resource{Type: "step_execution", ID: step.Name},
	})

	// Resolve CEL expressions in params.
	resolvedParams, err := e.CEL.ResolveParams(step.Params, celCtx)
	if err != nil {
		errMsg := fmt.Sprintf("resolving params: %v", err)
		if cpErr := e.updateStep(ctx, execID, step.Name, "failed", nil, errMsg); cpErr != nil {
			return StepResult{Status: "failed", Error: fmt.Sprintf("checkpoint failure: %v (original: %s)", cpErr, errMsg)}
		}
		return StepResult{Status: "failed", Error: errMsg}
	}

	// Resolve credential if specified.
	if step.Credential != "" {
		credData, credErr := e.Resolver.Resolve(ctx, step.Credential)
		if credErr != nil {
			errMsg := fmt.Sprintf("resolving credential %q: %v", step.Credential, credErr)
			if cpErr := e.updateStep(ctx, execID, step.Name, "failed", nil, errMsg); cpErr != nil {
				return StepResult{Status: "failed", Error: fmt.Sprintf("checkpoint failure: %v (original: %s)", cpErr, errMsg)}
			}
			return StepResult{Status: "failed", Error: errMsg}
		}
		resolvedParams["_credential"] = credData
	}

	// Look up connector.
	conn, err := e.Registry.Get(step.Action)
	if err != nil {
		errMsg := fmt.Sprintf("unknown action: %v", err)
		if cpErr := e.updateStep(ctx, execID, step.Name, "failed", nil, errMsg); cpErr != nil {
			return StepResult{Status: "failed", Error: fmt.Sprintf("checkpoint failure: %v (original: %s)", cpErr, errMsg)}
		}
		return StepResult{Status: "failed", Error: errMsg}
	}

	// Execute with retry and timeout.
	maxAttempts := 1
	if step.Retry != nil && step.Retry.MaxAttempts > 0 {
		maxAttempts = step.Retry.MaxAttempts
	}

	var output map[string]any
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Apply per-step timeout.
		execCtx := ctx
		var cancel context.CancelFunc
		if step.Timeout != "" {
			if d, parseErr := time.ParseDuration(step.Timeout); parseErr == nil {
				execCtx, cancel = context.WithTimeout(ctx, d)
			}
		}

		output, lastErr = conn.Execute(execCtx, resolvedParams)
		if cancel != nil {
			cancel()
		}

		if lastErr == nil {
			break
		}

		// If more attempts remain, apply backoff.
		if attempt < maxAttempts {
			backoff := time.Second // fixed default
			if step.Retry != nil && step.Retry.Backoff == "exponential" {
				backoff = time.Duration(1<<uint(attempt-1)) * time.Second
			}
			select {
			case <-ctx.Done():
				lastErr = ctx.Err()
				break
			case <-time.After(backoff):
			}
		}
	}

	if lastErr != nil {
		errMsg := lastErr.Error()
		if cpErr := e.updateStep(ctx, execID, step.Name, "failed", output, errMsg); cpErr != nil {
			return StepResult{Status: "failed", Output: output, Error: fmt.Sprintf("checkpoint failure: %v (original: %s)", cpErr, errMsg)}
		}
		e.Auditor.Emit(ctx, audit.Event{
			Timestamp: time.Now(),
			Actor:     "engine",
			Action:    audit.ActionStepFailed,
			Resource:  audit.Resource{Type: "step_execution", ID: step.Name},
		})
		return StepResult{Status: "failed", Output: output, Error: errMsg}
	}

	// Checkpoint: record completed step with output.
	if cpErr := e.updateStep(ctx, execID, step.Name, "completed", output, ""); cpErr != nil {
		return StepResult{Status: "failed", Error: fmt.Sprintf("checkpoint failure: %v", cpErr)}
	}
	e.Auditor.Emit(ctx, audit.Event{
		Timestamp: time.Now(),
		Actor:     "engine",
		Action:    audit.ActionStepCompleted,
		Resource:  audit.Resource{Type: "step_execution", ID: step.Name},
	})

	return StepResult{Status: "completed", Output: output}
}

// loadWorkflow retrieves a workflow definition from the database.
func (e *Engine) loadWorkflow(ctx context.Context, name string, version int) (*workflow.Workflow, error) {
	var content []byte
	err := e.DB.QueryRowContext(ctx,
		`SELECT content FROM workflow_definitions WHERE name = $1 AND version = $2`,
		name, version,
	).Scan(&content)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workflow %q version %d not found", name, version)
	}
	if err != nil {
		return nil, err
	}

	var wf workflow.Workflow
	if err := json.Unmarshal(content, &wf); err != nil {
		return nil, fmt.Errorf("unmarshaling workflow: %w", err)
	}
	return &wf, nil
}

// createExecution inserts a new workflow_executions row and returns the ID.
func (e *Engine) createExecution(ctx context.Context, workflowName string, version int, inputs map[string]any) (string, error) {
	inputsJSON, err := json.Marshal(inputs)
	if err != nil {
		return "", fmt.Errorf("marshaling inputs: %w", err)
	}

	var id string
	err = e.DB.QueryRowContext(ctx,
		`INSERT INTO workflow_executions (workflow_name, workflow_version, status, inputs, started_at)
		 VALUES ($1, $2, 'pending', $3, NOW())
		 RETURNING id`,
		workflowName, version, inputsJSON,
	).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}

// updateExecutionStatus updates the status of a workflow execution.
func (e *Engine) updateExecutionStatus(ctx context.Context, execID, status, errMsg string) error {
	var completedAt any
	if status == "completed" || status == "failed" || status == "cancelled" {
		completedAt = time.Now()
	}

	_, err := e.DB.ExecContext(ctx,
		`UPDATE workflow_executions SET status = $1, completed_at = $2, updated_at = NOW() WHERE id = $3`,
		status, completedAt, execID,
	)
	if err != nil {
		return fmt.Errorf("updating execution %s status to %s: %w", execID, status, err)
	}
	return nil
}

// recordStep inserts a new step_executions row.
func (e *Engine) recordStep(ctx context.Context, execID, stepName, status string, output map[string]any, errMsg string) error {
	outputJSON, err := json.Marshal(output)
	if err != nil {
		return fmt.Errorf("marshaling step %s output: %w", stepName, err)
	}

	var errorVal *string
	if errMsg != "" {
		errorVal = &errMsg
	}

	_, err = e.DB.ExecContext(ctx,
		`INSERT INTO step_executions (execution_id, step_name, status, output, error, started_at)
		 VALUES ($1, $2, $3, $4, $5, NOW())
		 ON CONFLICT (execution_id, step_name, attempt) DO NOTHING`,
		execID, stepName, status, outputJSON, errorVal,
	)
	if err != nil {
		return fmt.Errorf("recording step %s: %w", stepName, err)
	}
	return nil
}

// updateStep updates an existing step_executions row.
func (e *Engine) updateStep(ctx context.Context, execID, stepName, status string, output map[string]any, errMsg string) error {
	outputJSON, err := json.Marshal(output)
	if err != nil {
		return fmt.Errorf("marshaling step %s output: %w", stepName, err)
	}

	var errorVal *string
	if errMsg != "" {
		errorVal = &errMsg
	}

	var completedAt any
	if status == "completed" || status == "failed" || status == "skipped" {
		completedAt = time.Now()
	}

	_, err = e.DB.ExecContext(ctx,
		`UPDATE step_executions SET status = $1, output = $2, error = $3, completed_at = $4, updated_at = NOW()
		 WHERE execution_id = $5 AND step_name = $6 AND attempt = 1`,
		status, outputJSON, errorVal, completedAt, execID, stepName,
	)
	if err != nil {
		return fmt.Errorf("updating step %s: %w", stepName, err)
	}
	return nil
}

// loadCompletedSteps loads outputs of already-completed steps for checkpoint recovery.
func (e *Engine) loadCompletedSteps(ctx context.Context, execID string) (map[string]map[string]any, error) {
	rows, err := e.DB.QueryContext(ctx,
		`SELECT step_name, output FROM step_executions
		 WHERE execution_id = $1 AND status = 'completed'`,
		execID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]map[string]any)
	for rows.Next() {
		var name string
		var outputJSON []byte
		if err := rows.Scan(&name, &outputJSON); err != nil {
			return nil, err
		}
		var output map[string]any
		if err := json.Unmarshal(outputJSON, &output); err != nil {
			return nil, fmt.Errorf("unmarshaling step %q output: %w", name, err)
		}
		result[name] = output
	}
	return result, rows.Err()
}
