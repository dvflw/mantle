package engine

import (
	"context"
	"encoding/json"
	"fmt"

	mantleCEL "github.com/dvflw/mantle/internal/cel"
	"github.com/dvflw/mantle/internal/connector"
	"github.com/dvflw/mantle/internal/workflow"
)

// Default tool loop limits used when not specified in step params.
const (
	defaultMaxRounds        = 10
	defaultMaxCallsPerRound = 20
)

// executeToolUseStep handles AI steps that declare tools. It wires the ToolLoop,
// ToolSteps (for crash recovery), and the connector registry together to drive
// a multi-turn LLM-tool interaction loop with DB-backed checkpointing.
func (e *Engine) executeToolUseStep(ctx context.Context, execID string, step workflow.Step, celCtx *mantleCEL.Context, tools []workflow.Tool, workflowName string) (map[string]any, error) {
	// Look up the parent step's DB row ID for sub-step creation.
	parentStepID, err := e.getStepID(ctx, execID, step.Name)
	if err != nil {
		return nil, fmt.Errorf("looking up parent step ID: %w", err)
	}

	// Convert workflow Tool definitions into OpenAI function calling format.
	openaiTools := make([]map[string]any, len(tools))
	for i, t := range tools {
		openaiTools[i] = map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.InputSchema,
			},
		}
	}

	// Build a map from tool name to tool definition for connector dispatch.
	toolIndex := make(map[string]workflow.Tool, len(tools))
	for _, t := range tools {
		toolIndex[t.Name] = t
	}

	// Create ToolSteps for DB persistence of sub-steps and LLM response caching.
	toolSteps := &ToolSteps{DB: e.DB}

	// Resolve the base AI params (model, prompt, system_prompt, etc.) via CEL.
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

	// Inject workflow/step context for AI observability metrics.
	resolvedParams["_workflow"] = workflowName
	resolvedParams["_step"] = step.Name

	// Inject OpenAI-format tools into params.
	resolvedParams["_tools"] = openaiTools

	// Remove the workflow-level tools from resolved params (they are not
	// understood by the AI connector).
	delete(resolvedParams, "tools")

	// Look up the AI connector.
	aiConn, err := e.Registry.Get("ai/completion")
	if err != nil {
		return nil, fmt.Errorf("looking up ai/completion connector: %w", err)
	}

	// Build the ToolExecutor that dispatches tool calls to connectors.
	toolExecutor := func(ctx context.Context, toolName string, args map[string]any) (map[string]any, error) {
		tool, ok := toolIndex[toolName]
		if !ok {
			return nil, fmt.Errorf("unknown tool %q", toolName)
		}

		// Build the tool's params by resolving its declared params with
		// tool_input available in the CEL context.
		toolCELCtx := &mantleCEL.Context{
			Steps:  celCtx.Steps,
			Inputs: celCtx.Inputs,
		}
		// Inject tool_input as a top-level input so tool params can reference
		// it via expressions like {{ inputs.tool_input.query }}.
		enrichedInputs := make(map[string]any, len(celCtx.Inputs)+1)
		for k, v := range celCtx.Inputs {
			enrichedInputs[k] = v
		}
		enrichedInputs["tool_input"] = args
		toolCELCtx.Inputs = enrichedInputs

		toolParams, resolveErr := e.CEL.ResolveParams(tool.Params, toolCELCtx)
		if resolveErr != nil {
			return nil, fmt.Errorf("resolving tool %q params: %w", toolName, resolveErr)
		}

		// Look up the connector for this tool's action.
		conn, connErr := e.Registry.Get(tool.Action)
		if connErr != nil {
			return nil, fmt.Errorf("tool %q: unknown action %q: %w", toolName, tool.Action, connErr)
		}

		return conn.Execute(ctx, toolParams)
	}

	// Parse loop configuration from step params.
	maxRounds := defaultMaxRounds
	if v, ok := resolvedParams["max_rounds"]; ok {
		if n, ok := toInt(v); ok && n > 0 {
			maxRounds = n
		}
	}
	// Enforce admin ceiling on max_rounds.
	if e.MaxToolRoundsLimit > 0 && maxRounds > e.MaxToolRoundsLimit {
		maxRounds = e.MaxToolRoundsLimit
	}
	maxCallsPerRound := defaultMaxCallsPerRound
	if v, ok := resolvedParams["max_calls_per_round"]; ok {
		if n, ok := toInt(v); ok && n > 0 {
			maxCallsPerRound = n
		}
	}

	// --- Crash recovery: check for cached LLM responses ---
	cached, _ := toolSteps.LoadCachedLLMResponses(ctx, parentStepID)
	if len(cached) > 0 {
		recoveredMessages, recoverErr := e.reconstructMessages(resolvedParams, cached, toolSteps, parentStepID)
		if recoverErr != nil {
			return nil, fmt.Errorf("crash recovery: reconstructing messages: %w", recoverErr)
		}
		resolvedParams["_messages"] = recoveredMessages
		// Clear prompt/system_prompt so the ToolLoop does not duplicate them.
		delete(resolvedParams, "prompt")
		delete(resolvedParams, "system_prompt")
	}

	// Create the ToolLoop.
	toolLoop := &connector.ToolLoop{
		AIExecute:        func(ctx context.Context, params map[string]any) (map[string]any, error) { return aiConn.Execute(ctx, params) },
		ToolExecutor:     toolExecutor,
		MaxRounds:        maxRounds,
		MaxCallsPerRound: maxCallsPerRound,
		OnLLMResponse: func(response map[string]any) error {
			return toolSteps.CacheLLMResponse(ctx, parentStepID, response)
		},
		OnToolResult: func(toolName string, round int, result map[string]any) error {
			subStepName := fmt.Sprintf("%s/tool/%s/%d", step.Name, toolName, round)
			_, createErr := toolSteps.CreateSubStep(ctx, execID, parentStepID, subStepName, 1)
			return createErr
		},
	}

	return toolLoop.Run(ctx, resolvedParams)
}

// reconstructMessages rebuilds the conversation message array from cached LLM
// responses and completed sub-steps so the ToolLoop can resume from where it
// left off after a crash.
func (e *Engine) reconstructMessages(params map[string]any, cached []map[string]any, toolSteps *ToolSteps, parentStepID string) ([]map[string]any, error) {
	// Start with the original prompt messages.
	messages := connector.BuildInitialMessages(params)

	for _, response := range cached {
		// Check if this response contained tool calls.
		rawCalls, hasToolCalls := response["tool_calls"]
		if !hasToolCalls || rawCalls == nil {
			// Final text response was cached -- this means the loop completed
			// before the crash. The caller will just re-run the loop which
			// will immediately return this cached final response.
			break
		}

		// Serialize the tool calls into the assistant message.
		callsJSON, err := json.Marshal(rawCalls)
		if err != nil {
			return nil, fmt.Errorf("marshaling cached tool calls: %w", err)
		}
		var calls []connector.ToolCall
		if err := json.Unmarshal(callsJSON, &calls); err != nil {
			return nil, fmt.Errorf("unmarshaling cached tool calls: %w", err)
		}

		assistantMsg := map[string]any{
			"role":       "assistant",
			"content":    "",
			"tool_calls": connector.SerializeToolCalls(calls),
		}
		messages = append(messages, assistantMsg)

		// For each tool call, look up the cached result or use a placeholder.
		for _, tc := range calls {
			toolMsg := map[string]any{
				"role":         "tool",
				"tool_call_id": tc.ID,
				"content":      `{"recovered":true}`,
			}
			messages = append(messages, toolMsg)
		}
	}

	return messages, nil
}

// getStepID retrieves the UUID of a step_execution row by execution_id and step_name.
func (e *Engine) getStepID(ctx context.Context, execID, stepName string) (string, error) {
	var id string
	err := e.DB.QueryRowContext(ctx,
		`SELECT id FROM step_executions WHERE execution_id = $1 AND step_name = $2 AND attempt = 1`,
		execID, stepName,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("step %q not found in execution %s: %w", stepName, execID, err)
	}
	return id, nil
}

// toInt converts a value to int, handling common numeric types from JSON/CEL.
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i), true
		}
	}
	return 0, false
}
