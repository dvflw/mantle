package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dvflw/mantle/internal/metrics"
)

// ToolExecutor executes a named tool with the given arguments and returns the result.
type ToolExecutor func(ctx context.Context, toolName string, args map[string]any) (map[string]any, error)

// AIExecuteFunc calls the AI connector with the given params and returns the output.
type AIExecuteFunc func(ctx context.Context, params map[string]any) (map[string]any, error)

// ToolLoop orchestrates the multi-turn LLM-tool interaction loop. It repeatedly
// calls the LLM, dispatches any requested tool calls, appends results to the
// conversation, and continues until the LLM produces a final text response or
// the configured limits are reached.
type ToolLoop struct {
	AIExecute        AIExecuteFunc
	ToolExecutor     ToolExecutor
	MaxRounds        int
	MaxCallsPerRound int
	StepName         string                                          // used for metrics labels
	OnLLMResponse    func(response map[string]any) error           // for crash recovery caching
	OnToolResult     func(toolName string, round int, result map[string]any) error
}

// Run drives the LLM<->tool interaction loop to completion. It builds the
// initial message array from params, then iterates up to MaxRounds times
// calling the LLM and dispatching tool invocations. If the LLM returns a
// text response (no tool_calls), it is returned immediately. If MaxRounds
// is exhausted, a final LLM call is made requesting a best-effort text
// response without tools.
func (tl *ToolLoop) Run(ctx context.Context, params map[string]any) (map[string]any, error) {
	if tl.MaxRounds <= 0 {
		tl.MaxRounds = 10
	}
	if tl.MaxCallsPerRound <= 0 {
		tl.MaxCallsPerRound = 20
	}

	messages := BuildInitialMessages(params)
	baseParams := copyParams(params)
	delete(baseParams, "prompt")
	delete(baseParams, "system_prompt")
	delete(baseParams, "_messages")

	stepLabel := tl.StepName
	if stepLabel == "" {
		stepLabel = "unknown"
	}

	for round := 0; round < tl.MaxRounds; round++ {
		roundStart := time.Now()
		metrics.RecordToolRound(stepLabel)

		callParams := copyParams(baseParams)
		callParams["_messages"] = messages

		output, err := tl.AIExecute(ctx, callParams)
		if err != nil {
			return nil, fmt.Errorf("tool loop round %d: AI call failed: %w", round+1, err)
		}

		// Cache LLM response for crash recovery.
		if tl.OnLLMResponse != nil {
			if err := tl.OnLLMResponse(output); err != nil {
				return nil, fmt.Errorf("tool loop round %d: caching LLM response: %w", round+1, err)
			}
		}

		// If no tool calls, this is the final response.
		rawCalls, hasToolCalls := output["tool_calls"]
		if !hasToolCalls || rawCalls == nil {
			metrics.RecordToolRoundDuration(stepLabel, time.Since(roundStart))
			return output, nil
		}

		calls, ok := rawCalls.([]ToolCall)
		if !ok {
			metrics.RecordToolRoundDuration(stepLabel, time.Since(roundStart))
			return nil, fmt.Errorf("tool loop round %d: unexpected tool_calls type %T", round+1, rawCalls)
		}
		if len(calls) == 0 {
			metrics.RecordToolRoundDuration(stepLabel, time.Since(roundStart))
			return output, nil
		}

		// Enforce per-round call limit.
		if len(calls) > tl.MaxCallsPerRound {
			return nil, fmt.Errorf("tool loop round %d: LLM requested %d tool calls, exceeding max_calls_per_round (%d)", round+1, len(calls), tl.MaxCallsPerRound)
		}

		// Append the assistant message with tool_calls to the conversation.
		assistantMsg := map[string]any{
			"role":       "assistant",
			"content":    "",
			"tool_calls": SerializeToolCalls(calls),
		}
		messages = append(messages, assistantMsg)

		// Execute each tool call and append results.
		for _, tc := range calls {
			args, parseErr := parseToolArguments(tc.Function.Arguments)

			var resultContent string
			if parseErr != nil {
				// Send the parse error back to the LLM as the tool result
				// rather than crashing the loop.
				resultContent = fmt.Sprintf(`{"error":"invalid tool arguments: %s"}`, parseErr.Error())
				metrics.RecordToolCall(stepLabel, tc.Function.Name, "failed")
			} else {
				result, execErr := tl.ToolExecutor(ctx, tc.Function.Name, args)
				if execErr != nil {
					resultContent = fmt.Sprintf(`{"error":"%s"}`, execErr.Error())
					metrics.RecordToolCall(stepLabel, tc.Function.Name, "failed")
				} else {
					encoded, _ := json.Marshal(result)
					resultContent = string(encoded)
					metrics.RecordToolCall(stepLabel, tc.Function.Name, "completed")

					if tl.OnToolResult != nil {
						if err := tl.OnToolResult(tc.Function.Name, round+1, result); err != nil {
							return nil, fmt.Errorf("tool loop round %d: tool result callback: %w", round+1, err)
						}
					}
				}
			}

			toolMsg := map[string]any{
				"role":         "tool",
				"tool_call_id": tc.ID,
				"content":      resultContent,
			}
			messages = append(messages, toolMsg)
		}

		metrics.RecordToolRoundDuration(stepLabel, time.Since(roundStart))
	}

	// MaxRounds exhausted — make one final call asking the LLM to respond
	// without tools.
	finalParams := copyParams(baseParams)
	delete(finalParams, "_tools") // remove tools so LLM cannot call them
	messages = append(messages, map[string]any{
		"role":    "user",
		"content": "Tool use limit reached. Provide your best response with the information gathered so far.",
	})
	finalParams["_messages"] = messages

	output, err := tl.AIExecute(ctx, finalParams)
	if err != nil {
		return nil, fmt.Errorf("tool loop final call: AI call failed: %w", err)
	}

	if _, hasToolCalls := output["tool_calls"]; hasToolCalls {
		return nil, fmt.Errorf("tool use limit reached after %d rounds and LLM still requested tool calls", tl.MaxRounds)
	}

	return output, nil
}

// BuildInitialMessages constructs the initial message array from params.
// If _messages is already provided, it is used directly. Otherwise, system_prompt
// and prompt are assembled into the standard system + user message pair.
func BuildInitialMessages(params map[string]any) []map[string]any {
	if raw, ok := params["_messages"]; ok {
		if msgs, ok := raw.([]map[string]any); ok {
			// Return a copy to avoid mutating the caller's slice.
			out := make([]map[string]any, len(msgs))
			copy(out, msgs)
			return out
		}
	}

	var messages []map[string]any
	if systemPrompt, _ := params["system_prompt"].(string); systemPrompt != "" {
		messages = append(messages, map[string]any{"role": "system", "content": systemPrompt})
	}
	if prompt, _ := params["prompt"].(string); prompt != "" {
		messages = append(messages, map[string]any{"role": "user", "content": prompt})
	}
	return messages
}

// copyParams returns a shallow copy of the params map.
func copyParams(params map[string]any) map[string]any {
	out := make(map[string]any, len(params))
	for k, v := range params {
		out[k] = v
	}
	return out
}

// parseToolArguments parses the JSON-encoded arguments string into a map.
func parseToolArguments(argsJSON string) (map[string]any, error) {
	if argsJSON == "" {
		return map[string]any{}, nil
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return nil, fmt.Errorf("parsing JSON arguments: %w", err)
	}
	return args, nil
}

// SerializeToolCalls converts []ToolCall into []map[string]any for the
// OpenAI messages format.
func SerializeToolCalls(calls []ToolCall) []map[string]any {
	out := make([]map[string]any, len(calls))
	for i, tc := range calls {
		out[i] = map[string]any{
			"id":   tc.ID,
			"type": tc.Type,
			"function": map[string]any{
				"name":      tc.Function.Name,
				"arguments": tc.Function.Arguments,
			},
		}
	}
	return out
}
