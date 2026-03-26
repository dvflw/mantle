package connector

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPConnector_GET(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("X-Custom", "test-value")
		w.WriteHeader(200)
		w.Write([]byte(`{"message":"hello"}`))
	}))
	defer server.Close()

	c := &HTTPConnector{}
	output, err := c.Execute(context.Background(), map[string]any{
		"method": "GET",
		"url":    server.URL,
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if output["status"] != int64(200) {
		t.Errorf("status = %v, want 200", output["status"])
	}
	if output["body"] != `{"message":"hello"}` {
		t.Errorf("body = %v, want JSON", output["body"])
	}

	// Check parsed JSON.
	j, ok := output["json"].(map[string]any)
	if !ok {
		t.Fatalf("json = %T, want map[string]any", output["json"])
	}
	if j["message"] != "hello" {
		t.Errorf("json.message = %v, want %q", j["message"], "hello")
	}

	// Check headers.
	headers, ok := output["headers"].(map[string]any)
	if !ok {
		t.Fatalf("headers = %T, want map[string]any", output["headers"])
	}
	if headers["x-custom"] != "test-value" {
		t.Errorf("x-custom header = %v, want %q", headers["x-custom"], "test-value")
	}
}

func TestHTTPConnector_POST(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected JSON content type, got %s", r.Header.Get("Content-Type"))
		}

		body, _ := io.ReadAll(r.Body)
		var data map[string]any
		json.Unmarshal(body, &data)
		if data["key"] != "value" {
			t.Errorf("body.key = %v, want %q", data["key"], "value")
		}

		w.WriteHeader(201)
		w.Write([]byte(`{"created":true}`))
	}))
	defer server.Close()

	c := &HTTPConnector{}
	output, err := c.Execute(context.Background(), map[string]any{
		"method": "POST",
		"url":    server.URL,
		"body":   map[string]any{"key": "value"},
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if output["status"] != int64(201) {
		t.Errorf("status = %v, want 201", output["status"])
	}
}

func TestHTTPConnector_CustomHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok123" {
			t.Errorf("Authorization = %q, want %q", r.Header.Get("Authorization"), "Bearer tok123")
		}
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	c := &HTTPConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"url": server.URL,
		"headers": map[string]any{
			"Authorization": "Bearer tok123",
		},
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
}

func TestHTTPConnector_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte("not found"))
	}))
	defer server.Close()

	c := &HTTPConnector{}
	output, err := c.Execute(context.Background(), map[string]any{
		"url": server.URL,
	})
	if err == nil {
		t.Fatal("Execute() expected error for 404, got nil")
	}
	// Output should still be populated even on error.
	if output["status"] != int64(404) {
		t.Errorf("status = %v, want 404", output["status"])
	}
}

func TestHTTPConnector_MissingURL(t *testing.T) {
	c := &HTTPConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"method": "GET",
	})
	if err == nil {
		t.Fatal("Execute() expected error for missing URL, got nil")
	}
}

func TestHTTPConnector_DefaultMethod(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected default GET, got %s", r.Method)
		}
		w.WriteHeader(200)
	}))
	defer server.Close()

	c := &HTTPConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"url": server.URL,
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
}

func TestRegistry_Get(t *testing.T) {
	r := NewRegistry()

	_, err := r.Get("http/request")
	if err != nil {
		t.Fatalf("Get(http/request) error: %v", err)
	}

	_, err = r.Get("unknown/action")
	if err == nil {
		t.Fatal("Get(unknown/action) expected error, got nil")
	}
}
