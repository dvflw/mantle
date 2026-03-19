package connector

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func mockOpenAIServer(t *testing.T, responseText string, statusCode int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected JSON content type, got %s", r.Header.Get("Content-Type"))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(map[string]any{
			"model": "gpt-4o",
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": responseText,
					},
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 20,
				"total_tokens":      30,
			},
		})
	}))
}

func TestAIConnector_BasicCompletion(t *testing.T) {
	server := mockOpenAIServer(t, "Hello! I'm an AI assistant.", 200)
	defer server.Close()

	c := &AIConnector{}
	output, err := c.Execute(context.Background(), map[string]any{
		"model":    "gpt-4o",
		"prompt":   "Say hello",
		"base_url": server.URL,
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if output["text"] != "Hello! I'm an AI assistant." {
		t.Errorf("text = %v, want %q", output["text"], "Hello! I'm an AI assistant.")
	}
	if output["model"] != "gpt-4o" {
		t.Errorf("model = %v, want %q", output["model"], "gpt-4o")
	}

	usage, ok := output["usage"].(map[string]any)
	if !ok {
		t.Fatalf("usage type = %T, want map[string]any", output["usage"])
	}
	if usage["total_tokens"] != 30 {
		t.Errorf("total_tokens = %v, want 30", usage["total_tokens"])
	}
}

func TestAIConnector_SystemPrompt(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"model":   "gpt-4o",
			"choices": []map[string]any{{"message": map[string]any{"content": "ok"}}},
			"usage":   map[string]any{"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0},
		})
	}))
	defer server.Close()

	c := &AIConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"model":         "gpt-4o",
		"prompt":        "Hello",
		"system_prompt": "You are a helpful assistant.",
		"base_url":      server.URL,
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	messages, ok := receivedBody["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("expected 2 messages (system + user), got %v", receivedBody["messages"])
	}
	first := messages[0].(map[string]any)
	if first["role"] != "system" {
		t.Errorf("first message role = %v, want %q", first["role"], "system")
	}
}

func TestAIConnector_StructuredOutput(t *testing.T) {
	jsonResponse := `{"summary":"test summary","score":8}`
	server := mockOpenAIServer(t, jsonResponse, 200)
	defer server.Close()

	c := &AIConnector{}
	output, err := c.Execute(context.Background(), map[string]any{
		"model":    "gpt-4o",
		"prompt":   "Summarize this",
		"base_url": server.URL,
		"output_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"summary": map[string]any{"type": "string"},
				"score":   map[string]any{"type": "number"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if output["text"] != jsonResponse {
		t.Errorf("text = %v, want %q", output["text"], jsonResponse)
	}

	// Structured output should be parsed into json field.
	parsed, ok := output["json"].(map[string]any)
	if !ok {
		t.Fatalf("json = %T, want map[string]any", output["json"])
	}
	if parsed["summary"] != "test summary" {
		t.Errorf("json.summary = %v, want %q", parsed["summary"], "test summary")
	}
}

func TestAIConnector_StructuredOutput_RequestFormat(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"model":   "gpt-4o",
			"choices": []map[string]any{{"message": map[string]any{"content": `{"result":"ok"}`}}},
			"usage":   map[string]any{"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0},
		})
	}))
	defer server.Close()

	c := &AIConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"model":    "gpt-4o",
		"prompt":   "Test",
		"base_url": server.URL,
		"output_schema": map[string]any{
			"type":       "object",
			"properties": map[string]any{"result": map[string]any{"type": "string"}},
		},
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	// Verify response_format was sent in the request.
	rf, ok := receivedBody["response_format"].(map[string]any)
	if !ok {
		t.Fatal("response_format not in request body")
	}
	if rf["type"] != "json_schema" {
		t.Errorf("response_format.type = %v, want %q", rf["type"], "json_schema")
	}
}

func TestAIConnector_Credential(t *testing.T) {
	var authHeader string
	var orgHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		orgHeader = r.Header.Get("OpenAI-Organization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"model":   "gpt-4o",
			"choices": []map[string]any{{"message": map[string]any{"content": "hi"}}},
			"usage":   map[string]any{"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0},
		})
	}))
	defer server.Close()

	c := &AIConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"model":    "gpt-4o",
		"prompt":   "Hello",
		"base_url": server.URL,
		"_credential": map[string]string{
			"api_key": "sk-test-123",
			"org_id":  "org-456",
		},
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if authHeader != "Bearer sk-test-123" {
		t.Errorf("Authorization = %q, want %q", authHeader, "Bearer sk-test-123")
	}
	if orgHeader != "org-456" {
		t.Errorf("OpenAI-Organization = %q, want %q", orgHeader, "org-456")
	}
}

func TestAIConnector_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":{"message":"Invalid API key"}}`))
	}))
	defer server.Close()

	c := &AIConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"model":    "gpt-4o",
		"prompt":   "Hello",
		"base_url": server.URL,
	})
	if err == nil {
		t.Fatal("Execute() expected error for 401, got nil")
	}
}

func TestAIConnector_MissingModel(t *testing.T) {
	c := &AIConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"prompt": "Hello",
	})
	if err == nil {
		t.Fatal("Execute() expected error for missing model, got nil")
	}
}

func TestAIConnector_MissingPrompt(t *testing.T) {
	c := &AIConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"model": "gpt-4o",
	})
	if err == nil {
		t.Fatal("Execute() expected error for missing prompt, got nil")
	}
}

func TestAIConnector_FunctionCallingRequest(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"model": "gpt-4o",
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": "",
						"tool_calls": []map[string]any{
							{
								"id":   "call_abc123",
								"type": "function",
								"function": map[string]any{
									"name":      "get_weather",
									"arguments": `{"location":"San Francisco","unit":"celsius"}`,
								},
							},
						},
					},
				},
			},
			"usage": map[string]any{"prompt_tokens": 15, "completion_tokens": 25, "total_tokens": 40},
		})
	}))
	defer server.Close()

	tools := []map[string]any{
		{
			"type": "function",
			"function": map[string]any{
				"name":        "get_weather",
				"description": "Get current weather",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"location": map[string]any{"type": "string"},
						"unit":     map[string]any{"type": "string", "enum": []string{"celsius", "fahrenheit"}},
					},
					"required": []string{"location"},
				},
			},
		},
	}

	c := &AIConnector{}
	output, err := c.Execute(context.Background(), map[string]any{
		"model":    "gpt-4o",
		"prompt":   "What is the weather in San Francisco?",
		"base_url": server.URL,
		"_tools":   tools,
		"_credential": map[string]string{
			"api_key": "test-key",
		},
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	// Verify tools were sent in the request body.
	if _, ok := receivedBody["tools"]; !ok {
		t.Fatal("expected 'tools' in request body")
	}
	reqTools, ok := receivedBody["tools"].([]any)
	if !ok || len(reqTools) != 1 {
		t.Fatalf("expected 1 tool in request, got %v", receivedBody["tools"])
	}

	// Verify tool_calls in output.
	if output["finish_reason"] != "tool_calls" {
		t.Errorf("finish_reason = %v, want %q", output["finish_reason"], "tool_calls")
	}
	toolCalls, ok := output["tool_calls"].([]toolCall)
	if !ok {
		t.Fatalf("tool_calls type = %T, want []toolCall", output["tool_calls"])
	}
	if len(toolCalls) != 1 {
		t.Fatalf("len(tool_calls) = %d, want 1", len(toolCalls))
	}
	if toolCalls[0].ID != "call_abc123" {
		t.Errorf("tool_calls[0].ID = %q, want %q", toolCalls[0].ID, "call_abc123")
	}
	if toolCalls[0].Type != "function" {
		t.Errorf("tool_calls[0].Type = %q, want %q", toolCalls[0].Type, "function")
	}
	if toolCalls[0].Function.Name != "get_weather" {
		t.Errorf("tool_calls[0].Function.Name = %q, want %q", toolCalls[0].Function.Name, "get_weather")
	}
	if toolCalls[0].Function.Arguments != `{"location":"San Francisco","unit":"celsius"}` {
		t.Errorf("tool_calls[0].Function.Arguments = %q", toolCalls[0].Function.Arguments)
	}

	// Text should not be set when tool_calls are returned.
	if _, hasText := output["text"]; hasText {
		t.Errorf("output should not have 'text' when tool_calls are returned, got %v", output["text"])
	}
}

func TestAIConnector_MessagesPassthrough(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"model":   "gpt-4o",
			"choices": []map[string]any{{"message": map[string]any{"content": "The weather is sunny."}}},
			"usage":   map[string]any{"prompt_tokens": 30, "completion_tokens": 10, "total_tokens": 40},
		})
	}))
	defer server.Close()

	// Simulate multi-turn: user message, assistant tool_call, tool result, then follow-up.
	multiTurnMessages := []map[string]any{
		{"role": "user", "content": "What is the weather?"},
		{
			"role":    "assistant",
			"content": "",
			"tool_calls": []map[string]any{
				{
					"id":   "call_abc123",
					"type": "function",
					"function": map[string]any{
						"name":      "get_weather",
						"arguments": `{"location":"NYC"}`,
					},
				},
			},
		},
		{
			"role":         "tool",
			"tool_call_id": "call_abc123",
			"content":      `{"temperature":72,"condition":"sunny"}`,
		},
	}

	c := &AIConnector{}
	output, err := c.Execute(context.Background(), map[string]any{
		"model":     "gpt-4o",
		"_messages": multiTurnMessages,
		"base_url":  server.URL,
		"_credential": map[string]string{
			"api_key": "test-key",
		},
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	// Verify _messages were sent as-is.
	msgs, ok := receivedBody["messages"].([]any)
	if !ok {
		t.Fatalf("messages type = %T, want []any", receivedBody["messages"])
	}
	if len(msgs) != 3 {
		t.Fatalf("len(messages) = %d, want 3", len(msgs))
	}
	// Check the tool role message was passed through.
	toolMsg := msgs[2].(map[string]any)
	if toolMsg["role"] != "tool" {
		t.Errorf("messages[2].role = %v, want %q", toolMsg["role"], "tool")
	}
	if toolMsg["tool_call_id"] != "call_abc123" {
		t.Errorf("messages[2].tool_call_id = %v, want %q", toolMsg["tool_call_id"], "call_abc123")
	}

	// Should get a normal text response.
	if output["finish_reason"] != "stop" {
		t.Errorf("finish_reason = %v, want %q", output["finish_reason"], "stop")
	}
	if output["text"] != "The weather is sunny." {
		t.Errorf("text = %v, want %q", output["text"], "The weather is sunny.")
	}

	// prompt should not be required when _messages is provided.
	// (already verified by not passing "prompt" above)
}

func TestAIConnector_FunctionCallingThenText(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")

		if callCount == 1 {
			// First call: return tool_calls.
			json.NewEncoder(w).Encode(map[string]any{
				"model": "gpt-4o",
				"choices": []map[string]any{
					{
						"message": map[string]any{
							"content": "",
							"tool_calls": []map[string]any{
								{
									"id":   "call_xyz789",
									"type": "function",
									"function": map[string]any{
										"name":      "search_db",
										"arguments": `{"query":"latest orders"}`,
									},
								},
							},
						},
					},
				},
				"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 15, "total_tokens": 25},
			})
		} else {
			// Second call: return text.
			json.NewEncoder(w).Encode(map[string]any{
				"model": "gpt-4o",
				"choices": []map[string]any{
					{
						"message": map[string]any{
							"content": "Here are the latest 5 orders.",
						},
					},
				},
				"usage": map[string]any{"prompt_tokens": 20, "completion_tokens": 10, "total_tokens": 30},
			})
		}
	}))
	defer server.Close()

	c := &AIConnector{}

	// First call: expect tool_calls.
	output1, err := c.Execute(context.Background(), map[string]any{
		"model":    "gpt-4o",
		"prompt":   "Show me recent orders",
		"base_url": server.URL,
		"_tools": []map[string]any{
			{
				"type": "function",
				"function": map[string]any{
					"name":        "search_db",
					"description": "Search the database",
				},
			},
		},
		"_credential": map[string]string{
			"api_key": "test-key",
		},
	})
	if err != nil {
		t.Fatalf("Execute() call 1 error: %v", err)
	}
	if output1["finish_reason"] != "tool_calls" {
		t.Errorf("call 1: finish_reason = %v, want %q", output1["finish_reason"], "tool_calls")
	}
	toolCalls, ok := output1["tool_calls"].([]toolCall)
	if !ok || len(toolCalls) != 1 {
		t.Fatalf("call 1: tool_calls = %v", output1["tool_calls"])
	}
	if toolCalls[0].Function.Name != "search_db" {
		t.Errorf("call 1: function name = %q, want %q", toolCalls[0].Function.Name, "search_db")
	}

	// Second call: provide tool results via _messages, expect text.
	output2, err := c.Execute(context.Background(), map[string]any{
		"model": "gpt-4o",
		"_messages": []map[string]any{
			{"role": "user", "content": "Show me recent orders"},
			{
				"role":    "assistant",
				"content": "",
				"tool_calls": []map[string]any{
					{
						"id":   "call_xyz789",
						"type": "function",
						"function": map[string]any{
							"name":      "search_db",
							"arguments": `{"query":"latest orders"}`,
						},
					},
				},
			},
			{
				"role":         "tool",
				"tool_call_id": "call_xyz789",
				"content":      `[{"id":1,"total":99.99},{"id":2,"total":149.50}]`,
			},
		},
		"base_url": server.URL,
		"_credential": map[string]string{
			"api_key": "test-key",
		},
	})
	if err != nil {
		t.Fatalf("Execute() call 2 error: %v", err)
	}
	if output2["finish_reason"] != "stop" {
		t.Errorf("call 2: finish_reason = %v, want %q", output2["finish_reason"], "stop")
	}
	if output2["text"] != "Here are the latest 5 orders." {
		t.Errorf("call 2: text = %v, want %q", output2["text"], "Here are the latest 5 orders.")
	}
}

func TestRegistry_AIConnector(t *testing.T) {
	r := NewRegistry()
	_, err := r.Get("ai/completion")
	if err != nil {
		t.Fatalf("Get(ai/completion) error: %v", err)
	}
}
