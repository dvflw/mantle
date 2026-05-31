package connector

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJiraCreateIssueConnector_CreatesIssue(t *testing.T) {
	var gotBody map[string]any
	var gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/rest/api/3/issue" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"id":   "10001",
			"key":  "PROJ-1",
			"self": "https://mycompany.atlassian.net/rest/api/3/issue/10001",
		})
	}))
	defer srv.Close()

	c := &JiraCreateIssueConnector{baseURL: srv.URL}
	out, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{
			"domain":    "mycompany.atlassian.net",
			"email":     "user@example.com",
			"api_token": "secret-token",
		},
		"project_key": "PROJ",
		"summary":     "Fix the bug",
		"issue_type":  "Bug",
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	// Basic auth should be set
	if gotAuth == "" {
		t.Error("expected Authorization header to be set")
	}

	fields, _ := gotBody["fields"].(map[string]any)
	if fields["summary"] != "Fix the bug" {
		t.Errorf("summary = %v", fields["summary"])
	}
	project, _ := fields["project"].(map[string]any)
	if project["key"] != "PROJ" {
		t.Errorf("project.key = %v", project["key"])
	}
	issuetype, _ := fields["issuetype"].(map[string]any)
	if issuetype["name"] != "Bug" {
		t.Errorf("issuetype.name = %v", issuetype["name"])
	}

	if out["key"] != "PROJ-1" {
		t.Errorf("key = %v, want PROJ-1", out["key"])
	}
}

func TestJiraCreateIssueConnector_DefaultsToTask(t *testing.T) {
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"id": "1", "key": "PROJ-1", "self": ""})
	}))
	defer srv.Close()

	c := &JiraCreateIssueConnector{baseURL: srv.URL}
	_, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{
			"domain": "mycompany.atlassian.net", "email": "u@e.com", "api_token": "tok",
		},
		"project_key": "PROJ",
		"summary":     "Do something",
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	fields, _ := gotBody["fields"].(map[string]any)
	issuetype, _ := fields["issuetype"].(map[string]any)
	if issuetype["name"] != "Task" {
		t.Errorf("expected default issue_type=Task, got %v", issuetype["name"])
	}
}

func TestJiraCreateIssueConnector_MissingProjectKey(t *testing.T) {
	c := &JiraCreateIssueConnector{baseURL: "http://unused"}
	_, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{
			"domain": "x.atlassian.net", "email": "u@e.com", "api_token": "tok",
		},
		"summary": "Test",
	})
	if err == nil {
		t.Fatal("expected error for missing project_key")
	}
}

func TestJiraCreateIssueConnector_MissingCredential(t *testing.T) {
	c := &JiraCreateIssueConnector{baseURL: "http://unused"}
	_, err := c.Execute(context.Background(), map[string]any{
		"project_key": "PROJ",
		"summary":     "Test",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestJiraSearchIssuesConnector_SearchesIssues(t *testing.T) {
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/rest/api/3/issue/search" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"issues": []any{
				map[string]any{"id": "10001", "key": "PROJ-1"},
				map[string]any{"id": "10002", "key": "PROJ-2"},
			},
			"total": 2,
		})
	}))
	defer srv.Close()

	c := &JiraSearchIssuesConnector{baseURL: srv.URL}
	out, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{
			"domain": "mycompany.atlassian.net", "email": "u@e.com", "api_token": "tok",
		},
		"jql": "project = PROJ AND status = Open",
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if out["count"].(int) != 2 {
		t.Errorf("expected count=2, got %v", out["count"])
	}
	if out["total"].(int) != 2 {
		t.Errorf("expected total=2, got %v", out["total"])
	}
	if gotBody["jql"] != "project = PROJ AND status = Open" {
		t.Errorf("jql = %v", gotBody["jql"])
	}
}

func TestJiraSearchIssuesConnector_MissingJQL(t *testing.T) {
	c := &JiraSearchIssuesConnector{baseURL: "http://unused"}
	_, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{
			"domain": "x.atlassian.net", "email": "u@e.com", "api_token": "tok",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing jql")
	}
}

func TestRegistry_JiraConnectors(t *testing.T) {
	r := NewRegistry()
	for _, action := range []string{"jira/create_issue", "jira/search_issues"} {
		if _, err := r.Get(action); err != nil {
			t.Errorf("%s not registered: %v", action, err)
		}
	}
}
