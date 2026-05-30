package connector

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGitHubCreateIssueConnector_CreatesIssue(t *testing.T) {
	var gotAuth, gotVersion string
	var gotBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotVersion = r.Header.Get("X-GitHub-Api-Version")
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &gotBody)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"number":   42,
			"html_url": "https://github.com/acme/repo/issues/42",
			"node_id":  "I_abc123",
			"state":    "open",
			"title":    "Bug: something is broken",
		})
	}))
	defer server.Close()

	c := &GitHubCreateIssueConnector{baseURL: server.URL}
	out, err := c.Execute(context.Background(), map[string]any{
		"owner":       "acme",
		"repo":        "repo",
		"title":       "Bug: something is broken",
		"body":        "Details here.",
		"labels":      []any{"bug", "urgent"},
		"_credential": map[string]string{"token": "ghp_test"},
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if gotAuth != "Bearer ghp_test" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer ghp_test")
	}
	if gotVersion != "2022-11-28" {
		t.Errorf("X-GitHub-Api-Version = %q, want %q", gotVersion, "2022-11-28")
	}
	if gotBody["title"] != "Bug: something is broken" {
		t.Errorf("body title = %q", gotBody["title"])
	}
	if out["number"] != 42 {
		t.Errorf("number = %v, want 42", out["number"])
	}
	if out["url"] != "https://github.com/acme/repo/issues/42" {
		t.Errorf("url = %q", out["url"])
	}
	if out["state"] != "open" {
		t.Errorf("state = %q, want open", out["state"])
	}
}

func TestGitHubCreateIssueConnector_MissingTitle(t *testing.T) {
	c := &GitHubCreateIssueConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"owner":       "acme",
		"repo":        "repo",
		"_credential": map[string]string{"token": "ghp_test"},
	})
	if err == nil {
		t.Fatal("expected error for missing title")
	}
}

func TestGitHubCreateIssueConnector_MissingCredential(t *testing.T) {
	c := &GitHubCreateIssueConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"owner": "acme",
		"repo":  "repo",
		"title": "Test",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestGitHubCreateIssueConnector_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`{"message":"Validation Failed"}`))
	}))
	defer server.Close()

	c := &GitHubCreateIssueConnector{baseURL: server.URL}
	_, err := c.Execute(context.Background(), map[string]any{
		"owner":       "acme",
		"repo":        "repo",
		"title":       "Test",
		"_credential": map[string]string{"token": "ghp_test"},
	})
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}

func TestGitHubCreateIssueConnector_CredentialDeleted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"number": 1, "html_url": "https://github.com/a/b/issues/1",
			"node_id": "I_1", "state": "open", "title": "T",
		})
	}))
	defer server.Close()

	params := map[string]any{
		"owner":       "acme",
		"repo":        "repo",
		"title":       "T",
		"_credential": map[string]string{"token": "ghp_test"},
	}
	c := &GitHubCreateIssueConnector{baseURL: server.URL}
	if _, err := c.Execute(context.Background(), params); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if _, exists := params["_credential"]; exists {
		t.Error("_credential should be deleted from params after execution")
	}
}

func TestGitHubDispatchConnector_DispatchesEvent(t *testing.T) {
	var gotBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := &GitHubDispatchConnector{baseURL: server.URL}
	out, err := c.Execute(context.Background(), map[string]any{
		"owner":          "acme",
		"repo":           "repo",
		"event_type":     "deploy",
		"client_payload": map[string]any{"env": "production"},
		"_credential":    map[string]string{"token": "ghp_test"},
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if gotBody["event_type"] != "deploy" {
		t.Errorf("event_type = %q, want deploy", gotBody["event_type"])
	}
	if payload, ok := gotBody["client_payload"].(map[string]any); !ok || payload["env"] != "production" {
		t.Errorf("client_payload = %v", gotBody["client_payload"])
	}
	if out["ok"] != true {
		t.Errorf("ok = %v, want true", out["ok"])
	}
}

func TestGitHubDispatchConnector_MissingEventType(t *testing.T) {
	c := &GitHubDispatchConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"owner":       "acme",
		"repo":        "repo",
		"_credential": map[string]string{"token": "ghp_test"},
	})
	if err == nil {
		t.Fatal("expected error for missing event_type")
	}
}

func TestGitHubDispatchConnector_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Not Found"}`))
	}))
	defer server.Close()

	c := &GitHubDispatchConnector{baseURL: server.URL}
	_, err := c.Execute(context.Background(), map[string]any{
		"owner":       "acme",
		"repo":        "missing-repo",
		"event_type":  "deploy",
		"_credential": map[string]string{"token": "ghp_test"},
	})
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}

func TestRegistry_GitHubConnectors(t *testing.T) {
	r := NewRegistry()
	for _, action := range []string{"github/create_issue", "github/dispatch"} {
		if _, err := r.Get(action); err != nil {
			t.Errorf("Get(%q) error: %v", action, err)
		}
	}
}
