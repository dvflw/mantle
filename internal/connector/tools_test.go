package connector

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// mockAIFunc creates a mock AIExecuteFunc that returns responses in sequence.
// Each call pops the next response from the list. If exhausted, it returns an error.
func mockAIFunc(responses []map[string]any) AIExecuteFunc {
	var mu sync.Mutex
	idx := 0
	return func(ctx context.Context, params map[string]any) (map[string]any, error) {
		mu.Lock()
		defer mu.Unlock()
		if idx >= len(responses) {
			return nil, fmt.Errorf("mock AI: no more responses (called %d times)", idx+1)
		}
		resp := responses[idx]
		idx++
		return resp, nil
	}
}

// mockToolExecutor creates a ToolExecutor that records calls and returns a fixed result.
func mockToolExecutor(result map[string]any) (ToolExecutor, *[]string) {
	var mu sync.Mutex
	var called []string
	executor := func(ctx context.Context, toolName string, args map[string]any) (map[string]any, error) {
		mu.Lock()
		defer mu.Unlock()
		called = append(called, toolName)
		return result, nil
	}
	return executor, &called
}

func TestToolLoop_SingleRound(t *testing.T) {
	// Round 1: LLM returns a tool call. Round 2: LLM returns text.
	aiResponses := []map[string]any{
		{
			"finish_reason": "tool_calls",
			"tool_calls": []ToolCall{
				{
					ID:   "call_001",
					Type: "function",
					Function: ToolFunction{
						Name:      "get_weather",
						Arguments: `{"location":"NYC"}`,
					},
				},
			},
		},
		{
			"finish_reason": "stop",
			"text":          "The weather in NYC is sunny, 72F.",
		},
	}

	toolResult := map[string]any{"temperature": 72, "condition": "sunny"}
	executor, calledTools := mockToolExecutor(toolResult)

	loop := &ToolLoop{
		AIExecute:        mockAIFunc(aiResponses),
		ToolExecutor:     executor,
		MaxRounds:        5,
		MaxCallsPerRound: 10,
	}

	output, err := loop.Run(context.Background(), map[string]any{
		"model":  "gpt-4o",
		"prompt": "What is the weather in NYC?",
		"_tools": []map[string]any{{"type": "function"}},
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if output["text"] != "The weather in NYC is sunny, 72F." {
		t.Errorf("text = %v, want %q", output["text"], "The weather in NYC is sunny, 72F.")
	}
	if output["finish_reason"] != "stop" {
		t.Errorf("finish_reason = %v, want %q", output["finish_reason"], "stop")
	}
	if len(*calledTools) != 1 {
		t.Errorf("tool calls = %d, want 1", len(*calledTools))
	}
	if (*calledTools)[0] != "get_weather" {
		t.Errorf("tool called = %q, want %q", (*calledTools)[0], "get_weather")
	}
}

func TestToolLoop_MultipleToolCallsPerRound(t *testing.T) {
	// LLM returns 3 tool calls in one response, then text.
	aiResponses := []map[string]any{
		{
			"finish_reason": "tool_calls",
			"tool_calls": []ToolCall{
				{ID: "call_a", Type: "function", Function: ToolFunction{Name: "tool_alpha", Arguments: `{}`}},
				{ID: "call_b", Type: "function", Function: ToolFunction{Name: "tool_beta", Arguments: `{"x":1}`}},
				{ID: "call_c", Type: "function", Function: ToolFunction{Name: "tool_gamma", Arguments: `{"y":"hello"}`}},
			},
		},
		{
			"finish_reason": "stop",
			"text":          "All three tools executed successfully.",
		},
	}

	toolResult := map[string]any{"status": "ok"}
	executor, calledTools := mockToolExecutor(toolResult)

	loop := &ToolLoop{
		AIExecute:        mockAIFunc(aiResponses),
		ToolExecutor:     executor,
		MaxRounds:        5,
		MaxCallsPerRound: 10,
	}

	output, err := loop.Run(context.Background(), map[string]any{
		"model":  "gpt-4o",
		"prompt": "Run all three tools",
		"_tools": []map[string]any{{"type": "function"}},
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if output["text"] != "All three tools executed successfully." {
		t.Errorf("text = %v, want expected response", output["text"])
	}
	if len(*calledTools) != 3 {
		t.Fatalf("tool calls = %d, want 3", len(*calledTools))
	}
	expected := []string{"tool_alpha", "tool_beta", "tool_gamma"}
	for i, name := range expected {
		if (*calledTools)[i] != name {
			t.Errorf("calledTools[%d] = %q, want %q", i, (*calledTools)[i], name)
		}
	}
}

func TestToolLoop_MaxRoundsEnforced(t *testing.T) {
	// LLM always returns tool calls — never stops on its own.
	toolCallResponse := map[string]any{
		"finish_reason": "tool_calls",
		"tool_calls": []ToolCall{
			{ID: "call_loop", Type: "function", Function: ToolFunction{Name: "looping_tool", Arguments: `{}`}},
		},
	}

	// MaxRounds=2 means 2 rounds of tool calls, then 1 final call.
	// The final call also returns tool_calls, so we expect an error.
	aiResponses := []map[string]any{
		toolCallResponse, // round 1
		toolCallResponse, // round 2
		toolCallResponse, // final call (still returns tool_calls -> error)
	}

	toolResult := map[string]any{"data": "result"}
	executor, _ := mockToolExecutor(toolResult)

	loop := &ToolLoop{
		AIExecute:        mockAIFunc(aiResponses),
		ToolExecutor:     executor,
		MaxRounds:        2,
		MaxCallsPerRound: 10,
	}

	_, err := loop.Run(context.Background(), map[string]any{
		"model":  "gpt-4o",
		"prompt": "Keep looping",
		"_tools": []map[string]any{{"type": "function"}},
	})
	if err == nil {
		t.Fatal("Run() expected error for max rounds exceeded, got nil")
	}
	if !strings.Contains(err.Error(), "tool use limit") {
		t.Errorf("error = %q, want it to contain 'tool use limit'", err.Error())
	}
}

func TestToolLoop_MaxCallsPerRoundExceeded(t *testing.T) {
	// LLM returns 5 tool calls but limit is 2.
	aiResponses := []map[string]any{
		{
			"finish_reason": "tool_calls",
			"tool_calls": []ToolCall{
				{ID: "c1", Type: "function", Function: ToolFunction{Name: "t1", Arguments: `{}`}},
				{ID: "c2", Type: "function", Function: ToolFunction{Name: "t2", Arguments: `{}`}},
				{ID: "c3", Type: "function", Function: ToolFunction{Name: "t3", Arguments: `{}`}},
				{ID: "c4", Type: "function", Function: ToolFunction{Name: "t4", Arguments: `{}`}},
				{ID: "c5", Type: "function", Function: ToolFunction{Name: "t5", Arguments: `{}`}},
			},
		},
	}

	toolResult := map[string]any{"ok": true}
	executor, calledTools := mockToolExecutor(toolResult)

	loop := &ToolLoop{
		AIExecute:        mockAIFunc(aiResponses),
		ToolExecutor:     executor,
		MaxRounds:        5,
		MaxCallsPerRound: 2, // only allow 2 per round
	}

	_, err := loop.Run(context.Background(), map[string]any{
		"model":  "gpt-4o",
		"prompt": "Call many tools",
		"_tools": []map[string]any{{"type": "function"}},
	})
	if err == nil {
		t.Fatal("Run() expected error for too many tool calls, got nil")
	}
	if !strings.Contains(err.Error(), "max_calls_per_round") {
		t.Errorf("error = %q, want it to contain 'max_calls_per_round'", err.Error())
	}
	// No tools should have been executed.
	if len(*calledTools) != 0 {
		t.Errorf("tools executed = %d, want 0 (should fail before execution)", len(*calledTools))
	}
}

func TestToolLoop_InvalidToolArguments(t *testing.T) {
	// LLM returns tool call with invalid JSON arguments. The loop should
	// send an error message back to the LLM as a tool result rather than
	// crashing. The second LLM call returns a text response.
	aiCallCount := 0
	var capturedMessages []map[string]any

	aiFunc := func(ctx context.Context, params map[string]any) (map[string]any, error) {
		aiCallCount++
		if aiCallCount == 1 {
			return map[string]any{
				"finish_reason": "tool_calls",
				"tool_calls": []ToolCall{
					{
						ID:   "call_bad",
						Type: "function",
						Function: ToolFunction{
							Name:      "broken_tool",
							Arguments: `{not valid json!!!`,
						},
					},
				},
			}, nil
		}
		// Capture messages on second call to verify error was sent back.
		if msgs, ok := params["_messages"].([]map[string]any); ok {
			capturedMessages = make([]map[string]any, len(msgs))
			copy(capturedMessages, msgs)
		}
		return map[string]any{
			"finish_reason": "stop",
			"text":          "I see the tool had invalid arguments. Here is my best answer.",
		}, nil
	}

	toolExecutorCalled := false
	executor := func(ctx context.Context, toolName string, args map[string]any) (map[string]any, error) {
		toolExecutorCalled = true
		return map[string]any{}, nil
	}

	loop := &ToolLoop{
		AIExecute:        aiFunc,
		ToolExecutor:     executor,
		MaxRounds:        5,
		MaxCallsPerRound: 10,
	}

	output, err := loop.Run(context.Background(), map[string]any{
		"model":  "gpt-4o",
		"prompt": "Use the broken tool",
		"_tools": []map[string]any{{"type": "function"}},
	})
	if err != nil {
		t.Fatalf("Run() should not crash on invalid args, got error: %v", err)
	}

	if output["text"] != "I see the tool had invalid arguments. Here is my best answer." {
		t.Errorf("text = %v, unexpected", output["text"])
	}

	// The tool executor should NOT have been called since args were unparseable.
	if toolExecutorCalled {
		t.Error("ToolExecutor was called despite invalid arguments")
	}

	// Verify the error was sent back to the LLM as a tool result message.
	if len(capturedMessages) == 0 {
		t.Fatal("no messages captured from second AI call")
	}
	// Find the tool result message.
	var toolResultMsg map[string]any
	for _, msg := range capturedMessages {
		if msg["role"] == "tool" {
			toolResultMsg = msg
			break
		}
	}
	if toolResultMsg == nil {
		t.Fatal("no tool result message found in conversation")
	}
	content, _ := toolResultMsg["content"].(string)
	if !strings.Contains(content, "invalid tool arguments") {
		t.Errorf("tool result content = %q, want it to contain 'invalid tool arguments'", content)
	}
	if toolResultMsg["tool_call_id"] != "call_bad" {
		t.Errorf("tool_call_id = %v, want %q", toolResultMsg["tool_call_id"], "call_bad")
	}
}

func TestToolLoop_MaxToolResultBytesTruncation(t *testing.T) {
	// Tool returns a very large result that should be truncated.
	largeResult := make(map[string]any)
	largeResult["data"] = strings.Repeat("x", 50000) // 50KB

	aiResponses := []map[string]any{
		{
			"finish_reason": "tool_calls",
			"tool_calls": []ToolCall{
				{ID: "call_big", Type: "function", Function: ToolFunction{Name: "big_tool", Arguments: `{}`}},
			},
		},
		{
			"finish_reason": "stop",
			"text":          "Done.",
		},
	}

	var capturedMessages []map[string]any
	callCount := 0
	aiFunc := func(ctx context.Context, params map[string]any) (map[string]any, error) {
		callCount++
		if callCount == 2 {
			if msgs, ok := params["_messages"].([]map[string]any); ok {
				capturedMessages = make([]map[string]any, len(msgs))
				copy(capturedMessages, msgs)
			}
		}
		if callCount-1 < len(aiResponses) {
			return aiResponses[callCount-1], nil
		}
		return nil, fmt.Errorf("unexpected call %d", callCount)
	}

	executor := func(ctx context.Context, toolName string, args map[string]any) (map[string]any, error) {
		return largeResult, nil
	}

	loop := &ToolLoop{
		AIExecute:          aiFunc,
		ToolExecutor:       executor,
		MaxRounds:          5,
		MaxCallsPerRound:   10,
		MaxToolResultBytes: 1000, // 1KB limit
	}

	output, err := loop.Run(context.Background(), map[string]any{
		"model":  "gpt-4o",
		"prompt": "Use big tool",
		"_tools": []map[string]any{{"type": "function"}},
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if output["text"] != "Done." {
		t.Errorf("text = %v, want %q", output["text"], "Done.")
	}

	// The tool result in the messages should have been truncated.
	var toolMsg map[string]any
	for _, msg := range capturedMessages {
		if msg["role"] == "tool" {
			toolMsg = msg
			break
		}
	}
	if toolMsg == nil {
		t.Fatal("no tool result message found")
	}
	content, _ := toolMsg["content"].(string)
	if len(content) > 1100 { // 1000 + "...[truncated]" overhead
		t.Errorf("tool result content length = %d, expected truncated to ~1000", len(content))
	}
	if !strings.Contains(content, "...[truncated]") {
		t.Errorf("tool result content should contain truncation marker, got %q", content[:min(100, len(content))])
	}
}

func TestToolLoop_MaxTokenBudgetExceeded(t *testing.T) {
	// Each AI response reports 500 total_tokens. With budget of 800, the second
	// round should abort.
	aiResponses := []map[string]any{
		{
			"finish_reason": "tool_calls",
			"tool_calls": []ToolCall{
				{ID: "call_1", Type: "function", Function: ToolFunction{Name: "tool1", Arguments: `{}`}},
			},
			"usage": map[string]any{
				"prompt_tokens":     300,
				"completion_tokens": 200,
				"total_tokens":      500,
			},
		},
		{
			"finish_reason": "tool_calls",
			"tool_calls": []ToolCall{
				{ID: "call_2", Type: "function", Function: ToolFunction{Name: "tool2", Arguments: `{}`}},
			},
			"usage": map[string]any{
				"prompt_tokens":     400,
				"completion_tokens": 200,
				"total_tokens":      600,
			},
		},
	}

	toolResult := map[string]any{"ok": true}
	executor, _ := mockToolExecutor(toolResult)

	loop := &ToolLoop{
		AIExecute:        mockAIFunc(aiResponses),
		ToolExecutor:     executor,
		MaxRounds:        10,
		MaxCallsPerRound: 10,
		MaxTokenBudget:   800, // 500 + 600 = 1100 > 800
	}

	_, err := loop.Run(context.Background(), map[string]any{
		"model":  "gpt-4o",
		"prompt": "Use tools",
		"_tools": []map[string]any{{"type": "function"}},
	})
	if err == nil {
		t.Fatal("Run() expected error for token budget exceeded, got nil")
	}
	if !strings.Contains(err.Error(), "token usage") {
		t.Errorf("error = %q, want it to contain 'token usage'", err.Error())
	}
	if !strings.Contains(err.Error(), "exceeded budget") {
		t.Errorf("error = %q, want it to contain 'exceeded budget'", err.Error())
	}
}

func TestToolLoop_MaxTokenBudgetZeroIsUnlimited(t *testing.T) {
	// With budget=0 (default), no token limit is enforced.
	aiResponses := []map[string]any{
		{
			"finish_reason": "tool_calls",
			"tool_calls": []ToolCall{
				{ID: "call_1", Type: "function", Function: ToolFunction{Name: "tool1", Arguments: `{}`}},
			},
			"usage": map[string]any{
				"total_tokens": 999999,
			},
		},
		{
			"finish_reason": "stop",
			"text":          "Done despite huge token usage.",
		},
	}

	toolResult := map[string]any{"ok": true}
	executor, _ := mockToolExecutor(toolResult)

	loop := &ToolLoop{
		AIExecute:        mockAIFunc(aiResponses),
		ToolExecutor:     executor,
		MaxRounds:        5,
		MaxCallsPerRound: 10,
		MaxTokenBudget:   0, // unlimited
	}

	output, err := loop.Run(context.Background(), map[string]any{
		"model":  "gpt-4o",
		"prompt": "Use tools",
		"_tools": []map[string]any{{"type": "function"}},
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if output["text"] != "Done despite huge token usage." {
		t.Errorf("text = %v, unexpected", output["text"])
	}
}

func TestToolLoop_MessageByteTruncation(t *testing.T) {
	// Generate tool results that exceed the message byte limit, triggering
	// truncation of older results.
	bigContent := strings.Repeat("A", 5000) // 5KB per result

	callCount := 0
	aiFunc := func(ctx context.Context, params map[string]any) (map[string]any, error) {
		callCount++
		if callCount <= 3 {
			return map[string]any{
				"finish_reason": "tool_calls",
				"tool_calls": []ToolCall{
					{ID: fmt.Sprintf("call_%d", callCount), Type: "function", Function: ToolFunction{
						Name: fmt.Sprintf("tool_%d", callCount), Arguments: `{}`,
					}},
				},
			}, nil
		}
		return map[string]any{
			"finish_reason": "stop",
			"text":          "Final answer.",
		}, nil
	}

	executor := func(ctx context.Context, toolName string, args map[string]any) (map[string]any, error) {
		return map[string]any{"data": bigContent}, nil
	}

	loop := &ToolLoop{
		AIExecute:        aiFunc,
		ToolExecutor:     executor,
		MaxRounds:        5,
		MaxCallsPerRound: 10,
		MaxMessageBytes:  10000, // 10KB limit — will be exceeded after 2 rounds of 5KB results
	}

	output, err := loop.Run(context.Background(), map[string]any{
		"model":  "gpt-4o",
		"prompt": "Use tools repeatedly",
		"_tools": []map[string]any{{"type": "function"}},
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if output["text"] != "Final answer." {
		t.Errorf("text = %v, want %q", output["text"], "Final answer.")
	}
}

func TestEstimateMessageBytes(t *testing.T) {
	msg := map[string]any{
		"role":    "tool",
		"content": "hello world",
	}
	size := estimateMessageBytes(msg)
	// "tool" (4) + "hello world" (11) = 15
	if size != 15 {
		t.Errorf("estimateMessageBytes = %d, want 15", size)
	}
}

func TestTruncateOlderToolResults(t *testing.T) {
	messages := []map[string]any{
		{"role": "user", "content": "hi"},
		{"role": "tool", "tool_call_id": "c1", "content": strings.Repeat("x", 1000)},
		{"role": "tool", "tool_call_id": "c2", "content": strings.Repeat("y", 1000)},
		{"role": "tool", "tool_call_id": "c3", "content": strings.Repeat("z", 1000)},
		{"role": "tool", "tool_call_id": "c4", "content": strings.Repeat("w", 1000)},
	}

	truncated := truncateOlderToolResults(messages, 500)
	if truncated == 0 {
		t.Fatal("expected some messages to be truncated")
	}

	// The last tool message (c4) should be kept intact (last 25% = 1 of 4).
	lastContent, _ := messages[4]["content"].(string)
	if !strings.HasPrefix(lastContent, "wwww") {
		t.Errorf("last tool message should be intact, got %q", lastContent[:min(20, len(lastContent))])
	}

	// At least the first tool message should be truncated.
	firstContent, _ := messages[1]["content"].(string)
	if strings.HasPrefix(firstContent, "xxxx") {
		t.Error("first tool message should have been truncated")
	}
}

func TestExtractInt(t *testing.T) {
	tests := []struct {
		input any
		want  int
		ok    bool
	}{
		{42, 42, true},
		{float64(100), 100, true},
		{int64(200), 200, true},
		{"not a number", 0, false},
		{nil, 0, false},
	}
	for _, tt := range tests {
		got, ok := extractInt(tt.input)
		if got != tt.want || ok != tt.ok {
			t.Errorf("extractInt(%v) = (%d, %v), want (%d, %v)", tt.input, got, ok, tt.want, tt.ok)
		}
	}
}

