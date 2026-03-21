package connector

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIProvider_BasicCompletion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("expected /chat/completions, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"model": "gpt-4o",
			"choices": []map[string]any{
				{"message": map[string]any{"content": "Hello from provider!"}},
			},
			"usage": map[string]any{"prompt_tokens": 5, "completion_tokens": 10, "total_tokens": 15},
		})
	}))
	defer server.Close()

	p := &OpenAIProvider{Client: server.Client(), BaseURL: server.URL}
	resp, err := p.ChatCompletion(context.Background(), &ChatRequest{
		Model: "gpt-4o",
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello"},
		},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}
	if resp.Text != "Hello from provider!" {
		t.Errorf("Text = %q, want %q", resp.Text, "Hello from provider!")
	}
	if resp.FinishReason != "stop" {
		t.Errorf("FinishReason = %q, want %q", resp.FinishReason, "stop")
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("TotalTokens = %d, want 15", resp.Usage.TotalTokens)
	}
	if resp.Model != "gpt-4o" {
		t.Errorf("Model = %q, want %q", resp.Model, "gpt-4o")
	}
}

func TestOpenAIProvider_ToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"model": "gpt-4o",
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": "",
						"tool_calls": []map[string]any{
							{
								"id":   "call_123",
								"type": "function",
								"function": map[string]any{
									"name":      "get_weather",
									"arguments": `{"city":"NYC"}`,
								},
							},
						},
					},
				},
			},
			"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 15, "total_tokens": 25},
		})
	}))
	defer server.Close()

	p := &OpenAIProvider{Client: server.Client(), BaseURL: server.URL}
	resp, err := p.ChatCompletion(context.Background(), &ChatRequest{
		Model: "gpt-4o",
		Messages: []ChatMessage{
			{Role: "user", Content: "What is the weather?"},
		},
		Tools: []ChatTool{
			{Name: "get_weather", Description: "Get weather", InputSchema: map[string]any{"type": "object"}},
		},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}
	if resp.FinishReason != "tool_calls" {
		t.Errorf("FinishReason = %q, want %q", resp.FinishReason, "tool_calls")
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Function.Name != "get_weather" {
		t.Errorf("ToolCalls[0].Function.Name = %q, want %q", resp.ToolCalls[0].Function.Name, "get_weather")
	}
}

func TestOpenAIProvider_StructuredOutput(t *testing.T) {
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

	p := &OpenAIProvider{Client: server.Client(), BaseURL: server.URL}
	_, err := p.ChatCompletion(context.Background(), &ChatRequest{
		Model:    "gpt-4o",
		Messages: []ChatMessage{{Role: "user", Content: "Test"}},
		OutputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"result": map[string]any{"type": "string"}},
		},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}

	// Verify response_format was sent.
	rf, ok := receivedBody["response_format"].(map[string]any)
	if !ok {
		t.Fatal("response_format not in request body")
	}
	if rf["type"] != "json_schema" {
		t.Errorf("response_format.type = %v, want %q", rf["type"], "json_schema")
	}
}

func TestOpenAIProvider_AuthHeader(t *testing.T) {
	var authHeader, orgHeader string
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

	p := &OpenAIProvider{Client: server.Client(), BaseURL: server.URL}
	_, err := p.ChatCompletion(context.Background(), &ChatRequest{
		Model:    "gpt-4o",
		Messages: []ChatMessage{{Role: "user", Content: "Hello"}},
		Credential: map[string]string{
			"api_key": "sk-test-key",
			"org_id":  "org-test",
		},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}

	if authHeader != "Bearer sk-test-key" {
		t.Errorf("Authorization = %q, want %q", authHeader, "Bearer sk-test-key")
	}
	if orgHeader != "org-test" {
		t.Errorf("OpenAI-Organization = %q, want %q", orgHeader, "org-test")
	}
}

func TestOpenAIProvider_ToolsRequestFormat(t *testing.T) {
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

	p := &OpenAIProvider{Client: server.Client(), BaseURL: server.URL}
	_, err := p.ChatCompletion(context.Background(), &ChatRequest{
		Model:    "gpt-4o",
		Messages: []ChatMessage{{Role: "user", Content: "Test"}},
		Tools: []ChatTool{
			{
				Name:        "search",
				Description: "Search for things",
				InputSchema: map[string]any{
					"type":       "object",
					"properties": map[string]any{"query": map[string]any{"type": "string"}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}

	// Verify tools were sent in OpenAI format.
	rawTools, ok := receivedBody["tools"].([]any)
	if !ok || len(rawTools) != 1 {
		t.Fatalf("expected 1 tool, got %v", receivedBody["tools"])
	}
	tool := rawTools[0].(map[string]any)
	if tool["type"] != "function" {
		t.Errorf("tool.type = %v, want %q", tool["type"], "function")
	}
	fn := tool["function"].(map[string]any)
	if fn["name"] != "search" {
		t.Errorf("tool.function.name = %v, want %q", fn["name"], "search")
	}
}

func TestOpenAIProvider_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		w.Write([]byte(`{"error":{"message":"Rate limited"}}`))
	}))
	defer server.Close()

	p := &OpenAIProvider{Client: server.Client(), BaseURL: server.URL}
	_, err := p.ChatCompletion(context.Background(), &ChatRequest{
		Model:    "gpt-4o",
		Messages: []ChatMessage{{Role: "user", Content: "Hello"}},
	})
	if err == nil {
		t.Fatal("expected error for 429, got nil")
	}
}
