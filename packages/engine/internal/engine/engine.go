package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dvflw/mantle/internal/artifact"
	"github.com/dvflw/mantle/internal/audit"
	"github.com/dvflw/mantle/internal/auth"
	"github.com/dvflw/mantle/internal/budget"
	mantleCEL "github.com/dvflw/mantle/internal/cel"
	"github.com/dvflw/mantle/internal/connector"
	"github.com/dvflw/mantle/internal/metrics"
	"github.com/dvflw/mantle/internal/secret"
	"github.com/dvflw/mantle/internal/workflow"
)

// Engine executes workflows by running steps sequentially with checkpoint-and-resume.
type Engine struct {
	DB                 *sql.DB
	Registry           *connector.Registry
	Auditor            audit.Emitter
	CEL                *mantleCEL.Evaluator
	Resolver           *secret.Resolver
	MaxToolRoundsLimit int                  // admin ceiling for max_rounds; 0 = no limit
	MaxWorkflowDepth   int                  // max nesting depth for workflow/run; 0 = use default (10)
	BudgetChecker      *budget.Checker      // nil = budget enforcement disabled
	BudgetStore        *budget.Store        // nil = token usage recording disabled
	ArtifactStore      *artifact.Store      // nil = artifact system disabled
	TmpStorage         artifact.TmpStorage  // nil = artifact system disabled
}

// StepContext carries workflow-level metadata needed by step execution.
type StepContext struct {
	WorkflowTokenBudget int64
	CompletedSteps      map[string]map[string]any
	TeamID              string
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
	ExecutionID string                `json:"execution_id"`
	Status      string                `json:"status"`         // "completed" or "failed"
	Error       string                `json:"error,omitempty"`
	Duration    time.Duration         `json:"duration"`
	Steps       map[string]StepResult `json:"steps"`
}

// StepResult holds the outcome of a single step.
type StepResult struct {
	Status   string         `json:"status"`           // "completed", "failed", "skipped"
	Output   map[string]any `json:"output,omitempty"` // step output data
	Error    string         `json:"error,omitempty"`
	Duration time.Duration  `json:"duration"`
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
	execStart := time.Now()

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

	// Apply workflow-level timeout if specified.
	if wf.Timeout != "" {
		dur, err := time.ParseDuration(wf.Timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid workflow timeout %q: %w", wf.Timeout, err)
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, dur)
		defer cancel()
	}

	// Apply default values for missing inputs.
	if inputs == nil {
		inputs = make(map[string]any)
	}
	for name, input := range wf.Inputs {
		if _, ok := inputs[name]; !ok && input.Default != nil {
			inputs[name] = input.Default
		}
	}

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
		celCtx.Steps[name] = map[string]any{"output": output, "error": nil}
	}

	// Load failed-continued steps for checkpoint recovery.
	// These steps failed with continue_on_error=true; their error/output must be
	// re-exposed so downstream CEL expressions evaluate correctly on resume.
	failedContinuedSteps, err := e.loadFailedContinuedSteps(ctx, execID)
	if err != nil {
		return nil, fmt.Errorf("loading failed-continued steps: %w", err)
	}
	for name, fcs := range failedContinuedSteps {
		celCtx.Steps[name] = map[string]any{"output": fcs.output, "error": fcs.errMsg}
	}

	// Load artifacts for CEL context.
	e.loadArtifactsIntoCELContext(ctx, execID, celCtx)

	result := &ExecutionResult{
		ExecutionID: execID,
		Status:      "completed",
		Steps:       make(map[string]StepResult),
	}

	// Update execution status to running.
	if err := e.updateExecutionStatus(ctx, execID, "running", ""); err != nil {
		return nil, fmt.Errorf("checkpoint: %w", err)
	}

	// Build StepContext for budget enforcement.
	sc := StepContext{
		WorkflowTokenBudget: wf.TokenBudget,
		CompletedSteps:      completedSteps,
		TeamID:              auth.TeamIDFromContext(ctx),
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

		// Skip steps that already failed with continue_on_error=true (checkpoint recovery).
		if fcs, done := failedContinuedSteps[step.Name]; done {
			result.Steps[step.Name] = StepResult{
				Status: "failed",
				Output: fcs.output,
				Error:  fcs.errMsg,
			}
			continue
		}

		stepStart := time.Now()
		stepResult := e.executeStep(ctx, execID, workflowName, step, celCtx, sc)
		stepResult.Duration = time.Since(stepStart)
		result.Steps[step.Name] = stepResult

		if stepResult.Status == "completed" {
			celCtx.Steps[step.Name] = map[string]any{"output": stepResult.Output, "error": nil}
			sc.CompletedSteps[step.Name] = stepResult.Output
			e.loadArtifactsIntoCELContext(ctx, execID, celCtx)
		} else if stepResult.Status == "skipped" {
			celCtx.Steps[step.Name] = map[string]any{"output": map[string]any{}, "error": nil}
		} else if stepResult.Status == "failed" {
			if step.ContinueOnError {
				// Step failed but workflow continues — expose error and partial output to downstream steps.
				celCtx.Steps[step.Name] = map[string]any{
					"output": stepResult.Output,
					"error":  stepResult.Error,
				}
				e.Auditor.Emit(ctx, audit.Event{
					Timestamp: time.Now(),
					Actor:     "engine",
					Action:    audit.ActionStepContinuedOnError,
					Resource:  audit.Resource{Type: "step_execution", ID: step.Name},
					Metadata:  map[string]string{"error": stepResult.Error},
				})
				metrics.StepsContinuedOnErrorTotal.WithLabelValues(workflowName, step.Name).Inc()
			} else {
				// Original behavior — halt execution.
				result.Status = "failed"
				result.Error = fmt.Sprintf("step %q failed: %s", step.Name, stepResult.Error)
				result.Duration = time.Since(execStart)
				if err := e.updateExecutionStatus(ctx, execID, "failed", result.Error); err != nil {
					return nil, fmt.Errorf("checkpoint: %w", err)
				}
				metrics.ExecutionsTotal.WithLabelValues(workflowName, "failed").Inc()
				metrics.ExecutionDuration.WithLabelValues(workflowName).Observe(time.Since(execStart).Seconds())
				return result, nil
			}
		}
	}

	if err := e.updateExecutionStatus(ctx, execID, "completed", ""); err != nil {
		return nil, fmt.Errorf("checkpoint: %w", err)
	}
	result.Duration = time.Since(execStart)
	metrics.ExecutionsTotal.WithLabelValues(workflowName, "completed").Inc()
	metrics.ExecutionDuration.WithLabelValues(workflowName).Observe(time.Since(execStart).Seconds())
	return result, nil
}

// defaultMaxOutputBytes is the maximum allowed size for step output (1 MB).
const defaultMaxOutputBytes = 1048576

// checkOutputSize validates that serialised step output does not exceed maxBytes.
func checkOutputSize(output map[string]any, maxBytes int) error {
	data, err := json.Marshal(output)
	if err != nil {
		return fmt.Errorf("marshal output for size check: %w", err)
	}
	if len(data) > maxBytes {
		return fmt.Errorf("step output exceeds %d byte limit (%d bytes); use the S3 connector to store large payloads and pass the key to downstream steps", maxBytes, len(data))
	}
	return nil
}

func (e *Engine) executeStep(ctx context.Context, execID string, workflowName string, step workflow.Step, celCtx *mantleCEL.Context, sc StepContext) StepResult {
	// Evaluate conditional.
	if step.If != "" {
		shouldRun, err := e.CEL.EvalBool(step.If, celCtx)
		if err != nil {
			if cpErr := e.recordStep(ctx, execID, step.Name, "failed", nil, err.Error(), step.ContinueOnError); cpErr != nil {
				return StepResult{Status: "failed", Error: fmt.Sprintf("checkpoint failure: %v (original: evaluating if condition: %v)", cpErr, err)}
			}
			return StepResult{Status: "failed", Error: fmt.Sprintf("evaluating if condition: %v", err)}
		}
		if !shouldRun {
			if cpErr := e.recordStep(ctx, execID, step.Name, "skipped", nil, "", step.ContinueOnError); cpErr != nil {
				metrics.StepsTotal.WithLabelValues(workflowName, step.Name, "failed").Inc()
				return StepResult{Status: "failed", Error: fmt.Sprintf("checkpoint failure: %v", cpErr)}
			}
			e.Auditor.Emit(ctx, audit.Event{
				Timestamp: time.Now(),
				Actor:     "engine",
				Action:    audit.ActionStepSkipped,
				Resource:  audit.Resource{Type: "step_execution", ID: step.Name},
			})
			metrics.StepsTotal.WithLabelValues(workflowName, step.Name, "skipped").Inc()
			return StepResult{Status: "skipped"}
		}
	}

	// Record step as running.
	if cpErr := e.recordStep(ctx, execID, step.Name, "running", nil, "", step.ContinueOnError); cpErr != nil {
		return StepResult{Status: "failed", Error: fmt.Sprintf("checkpoint failure: %v", cpErr)}
	}
	e.Auditor.Emit(ctx, audit.Event{
		Timestamp: time.Now(),
		Actor:     "engine",
		Action:    audit.ActionStepStarted,
		Resource:  audit.Resource{Type: "step_execution", ID: step.Name},
	})

	// Delegate to the shared step logic (connector lookup, CEL resolution, retry).
	output, lastErr := e.executeStepLogic(ctx, execID, step, celCtx, workflowName, sc)

	if lastErr != nil {
		errMsg := lastErr.Error()
		if cpErr := e.updateStep(ctx, execID, step.Name, "failed", output, errMsg, step.ContinueOnError); cpErr != nil {
			return StepResult{Status: "failed", Output: output, Error: fmt.Sprintf("checkpoint failure: %v (original: %s)", cpErr, errMsg)}
		}
		e.Auditor.Emit(ctx, audit.Event{
			Timestamp: time.Now(),
			Actor:     "engine",
			Action:    audit.ActionStepFailed,
			Resource:  audit.Resource{Type: "step_execution", ID: step.Name},
		})
		metrics.StepsTotal.WithLabelValues(workflowName, step.Name, "failed").Inc()
		return StepResult{Status: "failed", Output: output, Error: errMsg}
	}

	// Validate output size before persisting.
	if err := checkOutputSize(output, defaultMaxOutputBytes); err != nil {
		errMsg := err.Error()
		if cpErr := e.updateStep(ctx, execID, step.Name, "failed", nil, errMsg, step.ContinueOnError); cpErr != nil {
			return StepResult{Status: "failed", Error: fmt.Sprintf("checkpoint failure: %v (original: %s)", cpErr, errMsg)}
		}
		e.Auditor.Emit(ctx, audit.Event{
			Timestamp: time.Now(),
			Actor:     "engine",
			Action:    audit.ActionStepFailed,
			Resource:  audit.Resource{Type: "step_execution", ID: step.Name},
		})
		metrics.StepsTotal.WithLabelValues(workflowName, step.Name, "failed").Inc()
		return StepResult{Status: "failed", Error: errMsg}
	}

	// Checkpoint: record completed step with output.
	if cpErr := e.updateStep(ctx, execID, step.Name, "completed", output, "", step.ContinueOnError); cpErr != nil {
		return StepResult{Status: "failed", Error: fmt.Sprintf("checkpoint failure: %v", cpErr)}
	}
	e.Auditor.Emit(ctx, audit.Event{
		Timestamp: time.Now(),
		Actor:     "engine",
		Action:    audit.ActionStepCompleted,
		Resource:  audit.Resource{Type: "step_execution", ID: step.Name},
	})

	metrics.StepsTotal.WithLabelValues(workflowName, step.Name, "completed").Inc()
	return StepResult{Status: "completed", Output: output}
}

// executeStepLogic contains the core connector invocation logic: CEL param resolution,
// credential resolution, connector lookup, and retry/timeout handling. It is used by
// both the sequential executeStep path and the distributed MakeStepExecutor bridge.
func (e *Engine) executeStepLogic(ctx context.Context, execID string, step workflow.Step, celCtx *mantleCEL.Context, workflowName string, sc StepContext) (map[string]any, error) {
	// Budget enforcement — check before dispatching AI steps.
	if strings.HasPrefix(step.Action, "ai/") {
		provider := "openai"
		if p, ok := step.Params["provider"].(string); ok && p != "" {
			provider = p
		}

		// 1. Workflow execution budget
		if sc.WorkflowTokenBudget > 0 {
			var usedTokens int64
			for _, stepOutput := range sc.CompletedSteps {
				usedTokens += extractTotalTokens(stepOutput)
			}
			result := budget.CheckExecutionBudget(sc.WorkflowTokenBudget, usedTokens)
			if result.Blocked {
				e.Auditor.Emit(ctx, audit.Event{
					Timestamp: time.Now(),
					Actor:     "engine",
					Action:    audit.ActionBudgetExceeded,
					Resource:  audit.Resource{Type: "workflow_execution", ID: execID},
					Metadata: map[string]string{
						"blocked_by": result.BlockedBy,
						"message":    result.Message,
					},
					TeamID: sc.TeamID,
				})
				return nil, fmt.Errorf("%s", result.Message)
			}
		}

		// 2. Team+provider and global budget
		if e.BudgetChecker != nil && sc.TeamID != "" {
			result := e.BudgetChecker.Check(ctx, budget.CheckInput{
				TeamID:   sc.TeamID,
				Provider: provider,
			})
			if result.Blocked {
				e.Auditor.Emit(ctx, audit.Event{
					Timestamp: time.Now(),
					Actor:     "engine",
					Action:    audit.ActionBudgetExceeded,
					Resource:  audit.Resource{Type: "workflow_execution", ID: execID},
					Metadata: map[string]string{
						"blocked_by": result.BlockedBy,
						"message":    result.Message,
						"provider":   provider,
					},
					TeamID: sc.TeamID,
				})
				metrics.BudgetCheckTotal.WithLabelValues(sc.TeamID, provider, "blocked").Inc()
				return nil, fmt.Errorf("%s", result.Message)
			}
			if result.Warning {
				e.Auditor.Emit(ctx, audit.Event{
					Timestamp: time.Now(),
					Actor:     "engine",
					Action:    audit.ActionBudgetWarning,
					Resource:  audit.Resource{Type: "workflow_execution", ID: execID},
					Metadata: map[string]string{
						"message":  result.Message,
						"provider": provider,
					},
					TeamID: sc.TeamID,
				})
				metrics.BudgetCheckTotal.WithLabelValues(sc.TeamID, provider, "warning").Inc()
			} else {
				metrics.BudgetCheckTotal.WithLabelValues(sc.TeamID, provider, "pass").Inc()
			}
		}
	}

	// Resolve CEL expressions in params.
	resolvedParams, err := e.CEL.ResolveParams(step.Params, celCtx)
	if err != nil {
		return nil, fmt.Errorf("resolving params: %w", err)
	}

	// Resolve credential if specified.
	if step.Credential != "" {
		credData, credErr := e.Resolver.Resolve(ctx, step.Credential)
		if credErr != nil {
			return nil, fmt.Errorf("resolving credential %q: %w", step.Credential, credErr)
		}
		resolvedParams["_credential"] = credData
	}

	// Resolve registry credential for docker/run private image pulls.
	if step.RegistryCredential != "" {
		regCredData, regCredErr := e.Resolver.Resolve(ctx, step.RegistryCredential)
		if regCredErr != nil {
			return nil, fmt.Errorf("resolving registry credential %q: %w", step.RegistryCredential, regCredErr)
		}
		resolvedParams["_registry_credential"] = regCredData
	}

	// Validate artifact subsystem is configured if step declares artifacts.
	var artifactsDir string
	if len(step.Artifacts) > 0 {
		if e.TmpStorage == nil || e.ArtifactStore == nil {
			return nil, fmt.Errorf("step %q declares artifacts but artifact subsystem is not configured (set tmp storage in mantle.yaml)", step.Name)
		}
	}

	// Inject workflow/step context for AI observability metrics.
	resolvedParams["_workflow"] = workflowName
	resolvedParams["_step"] = step.Name

	// Check for AI tool use — delegate to tool-use loop if tools are declared.
	if step.Action == "ai/completion" {
		tools, _ := workflow.ParseTools(step.Params)
		if len(tools) > 0 {
			return e.executeToolUseStep(ctx, execID, step, celCtx, tools, workflowName)
		}
	}

	// Look up connector.
	conn, err := e.Registry.Get(step.Action)
	if err != nil {
		return nil, fmt.Errorf("unknown action: %w", err)
	}

	// Execute with retry and timeout.
	maxAttempts := 1
	if step.Retry != nil && step.Retry.MaxAttempts > 0 {
		maxAttempts = step.Retry.MaxAttempts
	}

	var output map[string]any
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Create a fresh artifacts scratch dir per attempt.
		if len(step.Artifacts) > 0 && e.TmpStorage != nil {
			if artifactsDir != "" {
				os.RemoveAll(artifactsDir) // clean previous attempt's scratch
			}
			var tmpErr error
			artifactsDir, tmpErr = os.MkdirTemp("", "mantle-artifacts-*")
			if tmpErr != nil {
				return nil, fmt.Errorf("creating artifacts dir: %v", tmpErr)
			}
			ctx = artifact.WithArtifactsDir(ctx, artifactsDir)
		}

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

	// Record token usage for budget tracking.
	if lastErr == nil && strings.HasPrefix(step.Action, "ai/") && e.BudgetStore != nil && sc.TeamID != "" && output != nil {
		promptTok, completionTok := extractTokenCounts(output)
		if promptTok > 0 || completionTok > 0 {
			provider := "openai"
			if p, ok := step.Params["provider"].(string); ok && p != "" {
				provider = p
			}
			model, _ := output["model"].(string)
			resetMode, resetDay := "calendar", 1
			if e.BudgetChecker != nil {
				resetMode = e.BudgetChecker.ResetMode
				resetDay = e.BudgetChecker.ResetDay
			}
			period := budget.CurrentPeriodStart(time.Now(), resetMode, resetDay)
			if err := e.BudgetStore.IncrementUsage(ctx, sc.TeamID, provider, model, period, promptTok, completionTok); err != nil {
				// Log but don't fail the step — token recording is best-effort.
				log.Printf("budget: failed to record token usage: %v", err)
			}
		}
	}

	// Persist declared artifacts to tmp storage.
	if lastErr == nil && len(step.Artifacts) > 0 && e.TmpStorage != nil && e.ArtifactStore != nil && artifactsDir != "" {
		type persistedArtifact struct {
			id  string
			url string
		}
		var persisted []persistedArtifact

		rollback := func() {
			for _, p := range persisted {
				if delErr := e.ArtifactStore.DeleteByID(ctx, p.id); delErr != nil {
					log.Printf("warning: rollback: failed to delete artifact metadata %q: %v", p.id, delErr)
				}
				if delErr := e.TmpStorage.Delete(ctx, p.url); delErr != nil {
					log.Printf("warning: rollback: failed to delete artifact blob %q: %v", p.url, delErr)
				}
			}
		}

		for _, artDecl := range step.Artifacts {
			relPath := filepath.Base(artDecl.Path)
			localPath := filepath.Join(artifactsDir, relPath)
			info, statErr := os.Lstat(localPath) // Lstat: don't follow symlinks (defense-in-depth)
			if statErr != nil {
				rollback()
				return nil, fmt.Errorf("declared artifact %q not found at %q: %v", artDecl.Name, localPath, statErr)
			}

			key := fmt.Sprintf("%s/%s/%s/%s", workflowName, execID, artDecl.Name, relPath)
			url, putErr := e.TmpStorage.Put(ctx, key, localPath)
			if putErr != nil {
				rollback()
				return nil, fmt.Errorf("persisting artifact %q: %v", artDecl.Name, putErr)
			}

			art := &artifact.Artifact{
				ExecutionID: execID,
				StepName:    step.Name,
				Name:        artDecl.Name,
				URL:         url,
				Size:        info.Size(),
			}
			if createErr := e.ArtifactStore.Create(ctx, art); createErr != nil {
				if delErr := e.TmpStorage.Delete(ctx, url); delErr != nil {
					log.Printf("warning: failed to clean up orphaned artifact blob %q: %v", url, delErr)
				}
				rollback()
				return nil, fmt.Errorf("recording artifact %q: %v", artDecl.Name, createErr)
			}

			persisted = append(persisted, persistedArtifact{id: art.ID, url: url})

			// Emit audit event for artifact persistence.
			if err := e.Auditor.Emit(ctx, audit.Event{
				Timestamp: time.Now(),
				Actor:     "engine",
				Action:    audit.ActionArtifactPersisted,
				Resource:  audit.Resource{Type: "execution_artifact", ID: execID},
				Metadata: map[string]string{
					"step":          step.Name,
					"artifact_name": artDecl.Name,
					"url":           url,
					"size":          fmt.Sprintf("%d", info.Size()),
				},
				TeamID: sc.TeamID,
			}); err != nil {
				log.Printf("warning: failed to emit artifact audit event: %v", err)
			}
		}
	}

	// Clean up the scratch dir now that artifacts have been persisted (or on error).
	if artifactsDir != "" {
		os.RemoveAll(artifactsDir)
	}

	return output, lastErr
}

// MakeStepExecutor creates a StepExecutor closure that bridges the engine's
// existing sequential execution machinery to the distributed worker model.
// The returned function resolves a step by name from the workflow, then
// delegates to executeStepLogic for connector invocation with CEL resolution,
// credential handling, retry, and timeout.
func (e *Engine) MakeStepExecutor(wf *workflow.Workflow, execID string, celCtx *mantleCEL.Context) StepExecutor {
	return func(ctx context.Context, stepName string, attempt int) (map[string]any, error) {
		step := wf.FindStep(stepName)
		if step == nil {
			return nil, fmt.Errorf("step %q not found in workflow", stepName)
		}
		sc := StepContext{
			WorkflowTokenBudget: wf.TokenBudget,
			TeamID:              auth.TeamIDFromContext(ctx),
			CompletedSteps:      make(map[string]map[string]any),
		}
		for name, stepData := range celCtx.Steps {
			if out, ok := stepData["output"].(map[string]any); ok {
				sc.CompletedSteps[name] = out
			}
		}

		// Load artifacts for CEL context.
		e.loadArtifactsIntoCELContext(ctx, execID, celCtx)

		output, err := e.executeStepLogic(ctx, execID, *step, celCtx, wf.Name, sc)
		if err != nil {
			return output, err
		}
		// Validate output size before returning to the worker.
		if sizeErr := checkOutputSize(output, defaultMaxOutputBytes); sizeErr != nil {
			return nil, sizeErr
		}
		return output, nil
	}
}

// MakeGlobalStepExecutor creates a StepExecutor that resolves the workflow and
// CEL context dynamically from the execution ID stored in the context. This is
// used by the distributed worker, which claims arbitrary steps across all
// executions and therefore cannot be pre-bound to a single workflow.
func (e *Engine) MakeGlobalStepExecutor() StepExecutor {
	return func(ctx context.Context, stepName string, attempt int) (map[string]any, error) {
		execID := ExecutionIDFromContext(ctx)
		if execID == "" {
			return nil, fmt.Errorf("no execution ID in context")
		}

		// Look up workflow name and version for this execution.
		teamID := auth.TeamIDFromContext(ctx)
		var workflowName string
		var workflowVersion int
		err := e.DB.QueryRowContext(ctx,
			`SELECT workflow_name, workflow_version FROM workflow_executions WHERE id = $1 AND team_id = $2`,
			execID, teamID,
		).Scan(&workflowName, &workflowVersion)
		if err != nil {
			return nil, fmt.Errorf("loading execution metadata for %s: %w", execID, err)
		}

		// Load workflow definition.
		wf, err := e.loadWorkflow(ctx, workflowName, workflowVersion)
		if err != nil {
			return nil, fmt.Errorf("loading workflow %s v%d: %w", workflowName, workflowVersion, err)
		}

		step := wf.FindStep(stepName)
		if step == nil {
			return nil, fmt.Errorf("step %q not found in workflow %s", stepName, workflowName)
		}

		// Build CEL context from completed steps and execution inputs.
		completedSteps, err := e.loadCompletedSteps(ctx, execID)
		if err != nil {
			return nil, fmt.Errorf("loading completed steps for %s: %w", execID, err)
		}

		var inputs map[string]any
		var inputsJSON []byte
		if scanErr := e.DB.QueryRowContext(ctx,
			`SELECT inputs FROM workflow_executions WHERE id = $1 AND team_id = $2`, execID, teamID,
		).Scan(&inputsJSON); scanErr == nil && len(inputsJSON) > 0 {
			json.Unmarshal(inputsJSON, &inputs) //nolint:errcheck // best-effort
		}

		celCtx := &mantleCEL.Context{
			Steps:  make(map[string]map[string]any),
			Inputs: inputs,
		}
		if celCtx.Inputs == nil {
			celCtx.Inputs = make(map[string]any)
		}
		for name, output := range completedSteps {
			celCtx.Steps[name] = map[string]any{"output": output}
		}

		// Load failed-but-continued steps for CEL context.
		failedContinuedSteps, err := e.loadFailedContinuedSteps(ctx, execID)
		if err != nil {
			return nil, fmt.Errorf("loading failed continued steps: %w", err)
		}
		for name, fcs := range failedContinuedSteps {
			celCtx.Steps[name] = map[string]any{
				"output": fcs.output,
				"error":  fcs.errMsg,
			}
		}

		// Load artifacts for CEL context.
		e.loadArtifactsIntoCELContext(ctx, execID, celCtx)

		sc := StepContext{
			WorkflowTokenBudget: wf.TokenBudget,
			CompletedSteps:      completedSteps,
			TeamID:              teamID,
		}
		output, err := e.executeStepLogic(ctx, execID, *step, celCtx, workflowName, sc)
		if err != nil {
			return output, err
		}

		// Validate output size before returning to the worker.
		if sizeErr := checkOutputSize(output, defaultMaxOutputBytes); sizeErr != nil {
			return nil, sizeErr
		}
		return output, nil
	}
}

// loadArtifactsIntoCELContext loads artifact metadata into the CEL context.
func (e *Engine) loadArtifactsIntoCELContext(ctx context.Context, execID string, celCtx *mantleCEL.Context) {
	if e.ArtifactStore == nil {
		return
	}
	arts, err := e.ArtifactStore.ListByExecution(ctx, execID)
	if err != nil {
		log.Printf("warning: loading artifacts for execution %s: %v", execID, err)
		return
	}
	celCtx.Artifacts = make(map[string]map[string]any, len(arts))
	for _, a := range arts {
		celCtx.Artifacts[a.Name] = map[string]any{
			"name": a.Name,
			"url":  a.URL,
			"size": a.Size,
		}
	}
}

// loadWorkflow retrieves a workflow definition from the database.
func (e *Engine) loadWorkflow(ctx context.Context, name string, version int) (*workflow.Workflow, error) {
	teamID := auth.TeamIDFromContext(ctx)
	var content []byte
	err := e.DB.QueryRowContext(ctx,
		`SELECT content FROM workflow_definitions WHERE name = $1 AND version = $2 AND team_id = $3`,
		name, version, teamID,
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

	teamID := auth.TeamIDFromContext(ctx)

	var id string
	err = e.DB.QueryRowContext(ctx,
		`INSERT INTO workflow_executions (workflow_name, workflow_version, status, inputs, started_at, team_id)
		 VALUES ($1, $2, 'pending', $3, NOW(), $4)
		 RETURNING id`,
		workflowName, version, inputsJSON, teamID,
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

	teamID := auth.TeamIDFromContext(ctx)
	_, err := e.DB.ExecContext(ctx,
		`UPDATE workflow_executions SET status = $1, completed_at = $2, updated_at = NOW() WHERE id = $3 AND team_id = $4`,
		status, completedAt, execID, teamID,
	)
	if err != nil {
		return fmt.Errorf("updating execution %s status to %s: %w", execID, status, err)
	}
	return nil
}

// recordStep inserts a new step_executions row.
func (e *Engine) recordStep(ctx context.Context, execID, stepName, status string, output map[string]any, errMsg string, continueOnError bool) error {
	outputJSON, err := json.Marshal(output)
	if err != nil {
		return fmt.Errorf("marshaling step %s output: %w", stepName, err)
	}

	var errorVal *string
	if errMsg != "" {
		errorVal = &errMsg
	}

	_, err = e.DB.ExecContext(ctx,
		`INSERT INTO step_executions (execution_id, step_name, status, output, error, continue_on_error, started_at)
		 VALUES ($1, $2, $3, $4, $5, $6, NOW())
		 ON CONFLICT (execution_id, step_name, attempt) DO NOTHING`,
		execID, stepName, status, outputJSON, errorVal, continueOnError,
	)
	if err != nil {
		return fmt.Errorf("recording step %s: %w", stepName, err)
	}
	return nil
}

// updateStep updates an existing step_executions row.
func (e *Engine) updateStep(ctx context.Context, execID, stepName, status string, output map[string]any, errMsg string, continueOnError bool) error {
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
		`UPDATE step_executions SET status = $1, output = $2, error = $3, completed_at = $4, continue_on_error = $5, updated_at = NOW()
		 WHERE execution_id = $6 AND step_name = $7 AND attempt = 1`,
		status, outputJSON, errorVal, completedAt, continueOnError, execID, stepName,
	)
	if err != nil {
		return fmt.Errorf("updating step %s: %w", stepName, err)
	}
	return nil
}

// loadFailedContinuedSteps loads steps that failed but had continue_on_error=true,
// for checkpoint recovery — their error and partial output must be re-exposed in CEL.
type failedContinuedStep struct {
	output map[string]any
	errMsg string
}

func (e *Engine) loadFailedContinuedSteps(ctx context.Context, execID string) (map[string]failedContinuedStep, error) {
	rows, err := e.DB.QueryContext(ctx,
		`SELECT step_name, COALESCE(output::text, '{}'), COALESCE(error, '')
		 FROM step_executions
		 WHERE execution_id = $1 AND status = 'failed' AND continue_on_error = true`,
		execID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]failedContinuedStep)
	for rows.Next() {
		var name, outputStr, errMsg string
		if err := rows.Scan(&name, &outputStr, &errMsg); err != nil {
			return nil, err
		}
		var output map[string]any
		if err := json.Unmarshal([]byte(outputStr), &output); err != nil {
			return nil, fmt.Errorf("unmarshaling step %q output: %w", name, err)
		}
		result[name] = failedContinuedStep{output: output, errMsg: errMsg}
	}
	return result, rows.Err()
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

func extractTokenCounts(output map[string]any) (prompt, completion int64) {
	usage, ok := output["usage"].(map[string]any)
	if !ok {
		return 0, 0
	}
	if v, ok := usage["prompt_tokens"]; ok {
		prompt = toInt64(v)
	}
	if v, ok := usage["completion_tokens"]; ok {
		completion = toInt64(v)
	}
	return prompt, completion
}

func extractTotalTokens(output map[string]any) int64 {
	usage, ok := output["usage"].(map[string]any)
	if !ok {
		return 0
	}
	return toInt64(usage["total_tokens"])
}

func toInt64(v any) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int64:
		return n
	case float64:
		return int64(n)
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return i
		}
		return 0
	default:
		return 0
	}
}
