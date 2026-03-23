package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/dvflw/mantle/internal/metrics"
)

const (
	// DefaultMaxMessageBytes is the default cumulative message size limit (128KB).
	DefaultMaxMessageBytes = 128 * 1024
	// DefaultMaxToolResultBytes is the default per-tool-result size limit (32KB).
	DefaultMaxToolResultBytes = 32 * 1024
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

	// MaxMessageBytes limits cumulative message size across rounds (default 128KB).
	// When exceeded, older tool results are truncated to summaries.
	MaxMessageBytes int
	// MaxTokenBudget limits total_tokens across all API responses. 0 means unlimited.
	MaxTokenBudget int
	// MaxToolResultBytes truncates individual tool results beyond this limit (default 32KB).
	MaxToolResultBytes int
	// CumulativeTokens is a shared counter across retries. When non-nil, token
	// usage is accumulated here instead of in a local variable, and checked
	// against MaxTokenBudget. This prevents multiplicative cost when a step
	// retries (retry × max_rounds × max_calls).
	CumulativeTokens *int64
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
	if tl.MaxMessageBytes <= 0 {
		tl.MaxMessageBytes = DefaultMaxMessageBytes
	}
	if tl.MaxToolResultBytes <= 0 {
		tl.MaxToolResultBytes = DefaultMaxToolResultBytes
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

	// Track cumulative bytes and tokens across rounds.
	cumulativeBytes := estimateMessagesBytes(messages)
	// Use shared cross-retry counter if provided, otherwise track locally.
	var localTokens int64
	tokenCounter := &localTokens
	if tl.CumulativeTokens != nil {
		tokenCounter = tl.CumulativeTokens
	}

	for round := 0; round < tl.MaxRounds; round++ {
		roundStart := time.Now()
		metrics.RecordToolRound(stepLabel)

		// If approaching the message byte limit, truncate older tool results.
		if cumulativeBytes > tl.MaxMessageBytes {
			truncated := truncateOlderToolResults(messages, tl.MaxMessageBytes)
			if truncated > 0 {
				slog.Warn("tool loop: truncated older tool results to stay within message byte limit",
					"step", stepLabel, "round", round+1, "truncated_messages", truncated,
					"limit_bytes", tl.MaxMessageBytes)
			}
			cumulativeBytes = estimateMessagesBytes(messages)
		}

		callParams := copyParams(baseParams)
		callParams["_messages"] = messages

		output, err := tl.AIExecute(ctx, callParams)
		if err != nil {
			return nil, fmt.Errorf("tool loop round %d: AI call failed: %w", round+1, err)
		}

		// Track token usage across rounds (and across retries if CumulativeTokens is set).
		if usage, ok := output["usage"].(map[string]any); ok {
			if totalTokens, ok := extractInt(usage["total_tokens"]); ok {
				*tokenCounter += int64(totalTokens)
			}
		}
		if tl.MaxTokenBudget > 0 && int(*tokenCounter) > tl.MaxTokenBudget {
			return nil, fmt.Errorf("tool loop round %d: cumulative token usage (%d) exceeded budget (%d)",
				round+1, *tokenCounter, tl.MaxTokenBudget)
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
		cumulativeBytes += estimateMessageBytes(assistantMsg)

		// Execute each tool call and append results.
		for _, tc := range calls {
			args, parseErr := parseToolArguments(tc.Function.Arguments)

			var resultContent string
			if parseErr != nil {
				// Send the parse error back to the LLM as the tool result
				// rather than crashing the loop.
				slog.Warn("tool argument parse failed", "tool", tc.Function.Name, "error", parseErr)
			resultContent = `{"error":"invalid tool arguments"}`
				metrics.RecordToolCall(stepLabel, tc.Function.Name, "failed")
			} else {
				result, execErr := tl.ToolExecutor(ctx, tc.Function.Name, args)
				if execErr != nil {
					slog.Warn("tool execution failed", "tool", tc.Function.Name, "error", execErr)
				resultContent = `{"error":"tool execution failed"}`
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

			// Truncate individual tool results that exceed MaxToolResultBytes.
			if len(resultContent) > tl.MaxToolResultBytes {
				slog.Warn("tool loop: truncating oversized tool result",
					"step", stepLabel, "round", round+1, "tool", tc.Function.Name,
					"original_bytes", len(resultContent), "limit_bytes", tl.MaxToolResultBytes)
				resultContent = resultContent[:tl.MaxToolResultBytes] + "...[truncated]"
			}

			toolMsg := map[string]any{
				"role":         "tool",
				"tool_call_id": tc.ID,
				"content":      resultContent,
			}
			messages = append(messages, toolMsg)
			cumulativeBytes += estimateMessageBytes(toolMsg)
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

// estimateMessageBytes returns a rough byte-size estimate for a single message.
func estimateMessageBytes(msg map[string]any) int {
	size := 0
	if content, ok := msg["content"].(string); ok {
		size += len(content)
	}
	if role, ok := msg["role"].(string); ok {
		size += len(role)
	}
	// Account for serialized tool_calls if present.
	if tc, ok := msg["tool_calls"]; ok {
		encoded, _ := json.Marshal(tc)
		size += len(encoded)
	}
	return size
}

// estimateMessagesBytes returns the total estimated byte size of all messages.
func estimateMessagesBytes(messages []map[string]any) int {
	total := 0
	for _, msg := range messages {
		total += estimateMessageBytes(msg)
	}
	return total
}

// truncateOlderToolResults replaces content of older tool-role messages with a
// summary placeholder, working from the oldest tool messages forward, until the
// total message size is within targetBytes. Returns the number of messages truncated.
// The most recent tool messages are kept intact.
func truncateOlderToolResults(messages []map[string]any, targetBytes int) int {
	const truncatedPlaceholder = `{"_truncated":true,"summary":"tool result truncated to reduce message size"}`
	truncated := 0

	// Find indices of all tool messages.
	var toolIndices []int
	for i, msg := range messages {
		if msg["role"] == "tool" {
			toolIndices = append(toolIndices, i)
		}
	}

	// Truncate from oldest tool messages, skipping the most recent batch.
	// Keep the last 25% of tool messages intact (at least 1).
	keepCount := len(toolIndices) / 4
	if keepCount < 1 {
		keepCount = 1
	}
	truncatableCount := len(toolIndices) - keepCount

	for i := 0; i < truncatableCount; i++ {
		idx := toolIndices[i]
		content, ok := messages[idx]["content"].(string)
		if !ok || content == truncatedPlaceholder {
			continue
		}
		if len(content) <= len(truncatedPlaceholder) {
			continue // already small enough
		}
		messages[idx]["content"] = truncatedPlaceholder
		truncated++

		if estimateMessagesBytes(messages) <= targetBytes {
			break
		}
	}

	return truncated
}

// extractInt attempts to extract an int from a value that may be int or float64
// (JSON unmarshalling often produces float64).
func extractInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		return int(n), true
	case int64:
		return int(n), true
	}
	return 0, false
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
