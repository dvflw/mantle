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

// WorkflowConnector implements connector.Connector for the "workflow/run" action.
// It lives in the engine package (not connector) to avoid circular imports,
// since it needs a reference to *Engine.
type WorkflowConnector struct {
	engine *Engine
}

// Execute runs a child workflow as part of workflow composition.
//
// Params:
//   - workflow (string, required): name of the child workflow
//   - version (int, optional): version to run; 0 or omitted = latest
//   - inputs (map[string]any, optional): input parameters for the child
//   - token_budget (int64, optional): token budget override for the child
func (wc *WorkflowConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	// Extract required workflow name.
	workflowName, ok := params["workflow"].(string)
	if !ok || workflowName == "" {
		return nil, fmt.Errorf("workflow/run: 'workflow' parameter is required and must be a string")
	}

	// Extract optional version (default: latest).
	version := 0
	if v, ok := params["version"]; ok {
		switch tv := v.(type) {
		case float64:
			version = int(tv)
		case int:
			version = tv
		case json.Number:
			i, err := tv.Int64()
			if err == nil {
				version = int(i)
			}
		}
	}

	// Extract optional inputs.
	var inputs map[string]any
	if inp, ok := params["inputs"].(map[string]any); ok {
		inputs = inp
	}

	teamID := auth.TeamIDFromContext(ctx)

	// Resolve latest version if not specified.
	if version == 0 {
		v, err := workflow.GetLatestVersion(ctx, wc.engine.DB, workflowName)
		if err != nil {
			return nil, fmt.Errorf("workflow/run: resolving latest version: %w", err)
		}
		if v == 0 {
			return nil, fmt.Errorf("workflow/run: workflow %q not found", workflowName)
		}
		version = v
	}

	// Determine parent execution context.
	parentExecID := ExecutionIDFromContext(ctx)
	if parentExecID == "" {
		return nil, fmt.Errorf("workflow/run: no parent execution ID in context")
	}

	// Look up parent step name from params (injected by engine step execution as "_step").
	parentStepName, _ := params["_step"].(string)

	// Check depth limit.
	maxDepth := wc.engine.MaxWorkflowDepth
	if maxDepth <= 0 {
		maxDepth = 10
	}

	var parentDepth int
	err := wc.engine.DB.QueryRowContext(ctx,
		`SELECT depth FROM workflow_executions WHERE id = $1 AND team_id = $2`,
		parentExecID, teamID,
	).Scan(&parentDepth)
	if err != nil {
		return nil, fmt.Errorf("workflow/run: querying parent depth: %w", err)
	}

	childDepth := parentDepth + 1
	if childDepth > maxDepth {
		return nil, fmt.Errorf("workflow/run: max workflow depth %d exceeded (current depth: %d)", maxDepth, childDepth)
	}

	// Checkpoint recovery: check for existing child execution.
	var existingChildID string
	err = wc.engine.DB.QueryRowContext(ctx,
		`SELECT id FROM workflow_executions
		 WHERE parent_execution_id = $1 AND parent_step_name = $2 AND team_id = $3
		 LIMIT 1`,
		parentExecID, parentStepName, teamID,
	).Scan(&existingChildID)

	var childExecID string

	if err == nil {
		// Existing child found — resume it.
		childExecID = existingChildID
	} else if err == sql.ErrNoRows {
		// Create new child execution with parent linkage.
		inputsJSON, jsonErr := json.Marshal(inputs)
		if jsonErr != nil {
			return nil, fmt.Errorf("workflow/run: marshaling inputs: %w", jsonErr)
		}

		err = wc.engine.DB.QueryRowContext(ctx,
			`INSERT INTO workflow_executions
			 (workflow_name, workflow_version, status, inputs, started_at, team_id,
			  parent_execution_id, parent_step_name, depth)
			 VALUES ($1, $2, 'pending', $3, NOW(), $4, $5, $6, $7)
			 RETURNING id`,
			workflowName, version, inputsJSON, teamID,
			parentExecID, parentStepName, childDepth,
		).Scan(&childExecID)
		if err != nil {
			return nil, fmt.Errorf("workflow/run: creating child execution: %w", err)
		}
	} else {
		return nil, fmt.Errorf("workflow/run: checking for existing child: %w", err)
	}

	// Run the child workflow.
	result, err := wc.engine.resumeExecution(ctx, childExecID, workflowName, version, inputs)
	if err != nil {
		return nil, fmt.Errorf("workflow/run: child execution failed: %w", err)
	}

	// Emit audit event.
	wc.engine.Auditor.Emit(ctx, audit.Event{
		Timestamp: time.Now(),
		Actor:     "engine",
		Action:    audit.ActionChildWorkflowExecuted,
		Resource:  audit.Resource{Type: "workflow_execution", ID: childExecID},
		Metadata: map[string]string{
			"parent_execution_id": parentExecID,
			"parent_step_name":    parentStepName,
			"child_workflow":      workflowName,
			"child_version":       fmt.Sprintf("%d", version),
			"child_status":        result.Status,
			"depth":               fmt.Sprintf("%d", childDepth),
		},
	})

	// Build output: {execution_id, status, steps: {step_name: {output: ...}}}
	stepsOutput := make(map[string]any, len(result.Steps))
	for name, sr := range result.Steps {
		stepMap := map[string]any{
			"output": sr.Output,
		}
		if sr.Error != "" {
			stepMap["error"] = sr.Error
		}
		stepsOutput[name] = stepMap
	}

	output := map[string]any{
		"execution_id": childExecID,
		"status":       result.Status,
		"steps":        stepsOutput,
	}

	if result.Status == "failed" {
		return output, fmt.Errorf("child workflow %q failed: %s", workflowName, result.Error)
	}

	return output, nil
}

// RegisterWorkflowConnector registers the workflow/run connector with the engine's registry.
func (e *Engine) RegisterWorkflowConnector() {
	e.Registry.Register("workflow/run", &WorkflowConnector{engine: e})
}
