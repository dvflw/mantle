package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/dvflw/mantle/internal/audit"
	mantleCEL "github.com/dvflw/mantle/internal/cel"
	"github.com/dvflw/mantle/internal/metrics"
	"github.com/dvflw/mantle/internal/workflow"
)

// defaultHookTimeout is applied when the configured hooks timeout is unparseable.
const defaultHookTimeout = 5 * time.Minute

// executeHooks runs lifecycle hooks after main workflow execution completes.
// Hook failures are best-effort — they never alter the workflow execution status.
func (e *Engine) executeHooks(
	ctx context.Context,
	execID string,
	workflowName string,
	wf *workflow.Workflow,
	mainStatus string, // "completed", "failed", or "timed_out"
	mainError string,
	failedStep string,
	celCtx *mantleCEL.Context,
	sc StepContext,
) {
	if wf.Hooks == nil {
		return
	}

	// Populate execution context for CEL expressions in hook steps.
	celCtx.Execution = map[string]any{
		"status":      mainStatus,
		"error":       mainError,
		"failed_step": failedStep,
		"failed_in":   "steps",
	}

	// Initialize hooks namespace for inter-hook step references.
	celCtx.Hooks = make(map[string]map[string]any)

	// Apply hooks-level timeout if configured.
	if wf.Hooks.Timeout != "" {
		dur, err := time.ParseDuration(wf.Hooks.Timeout)
		if err != nil {
			log.Printf("hooks: invalid timeout %q: %v — falling back to %s", wf.Hooks.Timeout, err, defaultHookTimeout)
			dur = defaultHookTimeout
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, dur)
		defer cancel()
	}

	// Execute conditional block: on_success if completed, on_failure if failed/timed_out.
	if mainStatus == "completed" && len(wf.Hooks.OnSuccess) > 0 {
		e.executeHookBlock(ctx, execID, workflowName, "on_success", wf.Hooks.OnSuccess, celCtx, sc)
	} else if (mainStatus == "failed" || mainStatus == "timed_out") && len(wf.Hooks.OnFailure) > 0 {
		e.executeHookBlock(ctx, execID, workflowName, "on_failure", wf.Hooks.OnFailure, celCtx, sc)
	}

	// Execute on_finish always.
	if len(wf.Hooks.OnFinish) > 0 {
		e.executeHookBlock(ctx, execID, workflowName, "on_finish", wf.Hooks.OnFinish, celCtx, sc)
	}
}

// executeHookBlock runs a single hook block (on_success, on_failure, or on_finish) sequentially.
func (e *Engine) executeHookBlock(
	ctx context.Context,
	execID string,
	workflowName string,
	blockName string,
	steps []workflow.Step,
	celCtx *mantleCEL.Context,
	sc StepContext,
) {
	for _, step := range steps {
		// Check context cancellation (hooks timeout or parent cancellation).
		if ctx.Err() != nil {
			log.Printf("hooks: %s block cancelled (context done): %v", blockName, ctx.Err())
			return
		}

		// Emit audit: hook step started.
		e.Auditor.Emit(ctx, audit.Event{
			Timestamp: time.Now(),
			Actor:     "engine",
			Action:    audit.ActionHookStepStarted,
			Resource:  audit.Resource{Type: "hook_step_execution", ID: step.Name},
			Metadata: map[string]string{
				"execution_id": execID,
				"hook_block":   blockName,
				"step":         step.Name,
			},
		})

		// Record hook step as running.
		if err := e.recordHookStep(ctx, execID, step.Name, blockName, "running", nil, ""); err != nil {
			log.Printf("hooks: failed to record running state for %s/%s: %v", blockName, step.Name, err)
		}

		// Execute the step using the shared step logic.
		output, execErr := e.executeStepLogic(ctx, execID, step, celCtx, workflowName, sc)

		// Determine outcome.
		if execErr != nil {
			errMsg := execErr.Error()

			// Record failure in DB.
			if err := e.recordHookStep(ctx, execID, step.Name, blockName, "failed", output, errMsg); err != nil {
				log.Printf("hooks: failed to record failure for %s/%s: %v", blockName, step.Name, err)
			}

			// Update CEL context with error info.
			celCtx.Hooks[step.Name] = map[string]any{
				"output": output,
				"error":  errMsg,
			}

			// Emit metrics.
			metrics.HookStepsTotal.WithLabelValues(workflowName, blockName, step.Name, "failed").Inc()

			// Emit audit: hook step failed.
			e.Auditor.Emit(ctx, audit.Event{
				Timestamp: time.Now(),
				Actor:     "engine",
				Action:    audit.ActionHookStepFailed,
				Resource:  audit.Resource{Type: "hook_step_execution", ID: step.Name},
				Metadata: map[string]string{
					"execution_id": execID,
					"hook_block":   blockName,
					"step":         step.Name,
					"error":        errMsg,
				},
			})

			// Update failed_in to track where the failure occurred.
			celCtx.Execution["failed_in"] = blockName

			if step.ContinueOnError {
				log.Printf("hooks: %s step %q failed (continue_on_error): %s", blockName, step.Name, errMsg)
				continue
			}
			log.Printf("hooks: %s step %q failed, halting block: %s", blockName, step.Name, errMsg)
			return
		}

		// Success path.
		if err := e.recordHookStep(ctx, execID, step.Name, blockName, "completed", output, ""); err != nil {
			log.Printf("hooks: failed to record completion for %s/%s: %v", blockName, step.Name, err)
		}

		// Update CEL context with output.
		celCtx.Hooks[step.Name] = map[string]any{
			"output": output,
			"error":  nil,
		}

		// Emit metrics.
		metrics.HookStepsTotal.WithLabelValues(workflowName, blockName, step.Name, "completed").Inc()

		// Emit audit: hook step completed.
		e.Auditor.Emit(ctx, audit.Event{
			Timestamp: time.Now(),
			Actor:     "engine",
			Action:    audit.ActionHookStepCompleted,
			Resource:  audit.Resource{Type: "hook_step_execution", ID: step.Name},
			Metadata: map[string]string{
				"execution_id": execID,
				"hook_block":   blockName,
				"step":         step.Name,
			},
		})
	}
}

// recordHookStep inserts or updates a hook step execution record in the database.
// The hook_block column disambiguates hook steps from main steps (and from each other),
// so step names are stored unprefixed.
func (e *Engine) recordHookStep(ctx context.Context, execID, stepName, hookBlock, status string, output map[string]any, errMsg string) error {
	outputJSON, err := json.Marshal(output)
	if err != nil {
		return fmt.Errorf("marshaling hook step %s output: %w", stepName, err)
	}

	var errorVal *string
	if errMsg != "" {
		errorVal = &errMsg
	}

	_, err = e.DB.ExecContext(ctx,
		`INSERT INTO step_executions (execution_id, step_name, attempt, status, output, error, hook_block, started_at)
		 VALUES ($1, $2, 1, $3, $4, $5, $6, NOW())
		 ON CONFLICT (execution_id, step_name, attempt, hook_block) WHERE hook_block IS NOT NULL DO UPDATE
		 SET status = EXCLUDED.status, output = EXCLUDED.output, error = EXCLUDED.error, updated_at = NOW()`,
		execID, stepName, status, outputJSON, errorVal, hookBlock,
	)
	if err != nil {
		return fmt.Errorf("recording hook step %s: %w", stepName, err)
	}
	return nil
}
