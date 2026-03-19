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

func TestRegistry_AIConnector(t *testing.T) {
	r := NewRegistry()
	_, err := r.Get("ai/completion")
	if err != nil {
		t.Fatalf("Get(ai/completion) error: %v", err)
	}
}
