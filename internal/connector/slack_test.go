package connector

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSlackSendConnector_SendMessage(t *testing.T) {
	var receivedBody map[string]string
	var authHeader string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected JSON content type, got %s", r.Header.Get("Content-Type"))
		}
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"ok":      true,
			"ts":      "1234567890.123456",
			"channel": "C12345",
		})
	}))
	defer server.Close()

	c := &SlackSendConnector{baseURL: server.URL}
	output, err := c.Execute(context.Background(), map[string]any{
		"channel":     "C12345",
		"text":        "Hello from Mantle!",
		"_credential": map[string]string{"token": "xoxb-test-token"},
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if authHeader != "Bearer xoxb-test-token" {
		t.Errorf("Authorization = %q, want %q", authHeader, "Bearer xoxb-test-token")
	}
	if receivedBody["channel"] != "C12345" {
		t.Errorf("channel = %q, want %q", receivedBody["channel"], "C12345")
	}
	if receivedBody["text"] != "Hello from Mantle!" {
		t.Errorf("text = %q, want %q", receivedBody["text"], "Hello from Mantle!")
	}
	if output["ok"] != true {
		t.Errorf("ok = %v, want true", output["ok"])
	}
	if output["ts"] != "1234567890.123456" {
		t.Errorf("ts = %v, want %q", output["ts"], "1234567890.123456")
	}
	if output["channel"] != "C12345" {
		t.Errorf("channel = %v, want %q", output["channel"], "C12345")
	}
}

func TestSlackSendConnector_MissingChannel(t *testing.T) {
	c := &SlackSendConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"text":        "Hello",
		"_credential": map[string]string{"token": "xoxb-test"},
	})
	if err == nil {
		t.Fatal("Execute() expected error for missing channel, got nil")
	}
}

func TestSlackSendConnector_MissingText(t *testing.T) {
	c := &SlackSendConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"channel":     "C12345",
		"_credential": map[string]string{"token": "xoxb-test"},
	})
	if err == nil {
		t.Fatal("Execute() expected error for missing text, got nil")
	}
}

func TestSlackSendConnector_MissingCredential(t *testing.T) {
	c := &SlackSendConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"channel": "C12345",
		"text":    "Hello",
	})
	if err == nil {
		t.Fatal("Execute() expected error for missing credential, got nil")
	}
}

func TestSlackSendConnector_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"ok":    false,
			"error": "channel_not_found",
		})
	}))
	defer server.Close()

	c := &SlackSendConnector{baseURL: server.URL}
	_, err := c.Execute(context.Background(), map[string]any{
		"channel":     "C99999",
		"text":        "Hello",
		"_credential": map[string]string{"token": "xoxb-test"},
	})
	if err == nil {
		t.Fatal("Execute() expected error for Slack API error, got nil")
	}
	if got := err.Error(); got != "slack/send: Slack API error: channel_not_found" {
		t.Errorf("error = %q, want Slack API error: channel_not_found", got)
	}
}

func TestSlackSendConnector_CredentialDeleted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "ts": "1", "channel": "C1"})
	}))
	defer server.Close()

	params := map[string]any{
		"channel":     "C1",
		"text":        "hi",
		"_credential": map[string]string{"token": "xoxb-test"},
	}

	c := &SlackSendConnector{baseURL: server.URL}
	_, err := c.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if _, exists := params["_credential"]; exists {
		t.Error("_credential should be deleted from params after execution")
	}
}

func TestSlackHistoryConnector_ReadHistory(t *testing.T) {
	var authHeader string
	var queryChannel string
	var queryLimit string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		queryChannel = r.URL.Query().Get("channel")
		queryLimit = r.URL.Query().Get("limit")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"messages": []map[string]any{
				{"type": "message", "text": "Hello", "user": "U123", "ts": "1234567890.100"},
				{"type": "message", "text": "World", "user": "U456", "ts": "1234567890.200"},
			},
		})
	}))
	defer server.Close()

	c := &SlackHistoryConnector{baseURL: server.URL}
	output, err := c.Execute(context.Background(), map[string]any{
		"channel":     "C12345",
		"limit":       float64(5),
		"_credential": map[string]string{"token": "xoxb-test-token"},
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if authHeader != "Bearer xoxb-test-token" {
		t.Errorf("Authorization = %q, want %q", authHeader, "Bearer xoxb-test-token")
	}
	if queryChannel != "C12345" {
		t.Errorf("channel param = %q, want %q", queryChannel, "C12345")
	}
	if queryLimit != "5" {
		t.Errorf("limit param = %q, want %q", queryLimit, "5")
	}
	if output["ok"] != true {
		t.Errorf("ok = %v, want true", output["ok"])
	}
	messages, ok := output["messages"].([]any)
	if !ok {
		t.Fatalf("messages type = %T, want []any", output["messages"])
	}
	if len(messages) != 2 {
		t.Errorf("len(messages) = %d, want 2", len(messages))
	}
}

func TestSlackHistoryConnector_DefaultLimit(t *testing.T) {
	var queryLimit string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queryLimit = r.URL.Query().Get("limit")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "messages": []map[string]any{}})
	}))
	defer server.Close()

	c := &SlackHistoryConnector{baseURL: server.URL}
	_, err := c.Execute(context.Background(), map[string]any{
		"channel":     "C12345",
		"_credential": map[string]string{"token": "xoxb-test"},
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if queryLimit != "10" {
		t.Errorf("default limit = %q, want %q", queryLimit, "10")
	}
}

func TestSlackHistoryConnector_MissingChannel(t *testing.T) {
	c := &SlackHistoryConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{"token": "xoxb-test"},
	})
	if err == nil {
		t.Fatal("Execute() expected error for missing channel, got nil")
	}
}

func TestSlackHistoryConnector_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"ok":    false,
			"error": "not_authed",
		})
	}))
	defer server.Close()

	c := &SlackHistoryConnector{baseURL: server.URL}
	_, err := c.Execute(context.Background(), map[string]any{
		"channel":     "C12345",
		"_credential": map[string]string{"token": "xoxb-bad"},
	})
	if err == nil {
		t.Fatal("Execute() expected error for Slack API error, got nil")
	}
	if got := err.Error(); got != "slack/history: Slack API error: not_authed" {
		t.Errorf("error = %q, want Slack API error: not_authed", got)
	}
}

func TestSlackHistoryConnector_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	c := &SlackHistoryConnector{baseURL: server.URL}
	_, err := c.Execute(context.Background(), map[string]any{
		"channel":     "C12345",
		"_credential": map[string]string{"token": "xoxb-test"},
	})
	if err == nil {
		t.Fatal("Execute() expected error for HTTP 500, got nil")
	}
}

func TestRegistry_SlackConnectors(t *testing.T) {
	r := NewRegistry()

	if _, err := r.Get("slack/send"); err != nil {
		t.Errorf("Get(slack/send) error: %v", err)
	}
	if _, err := r.Get("slack/history"); err != nil {
		t.Errorf("Get(slack/history) error: %v", err)
	}
}
