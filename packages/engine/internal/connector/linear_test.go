package connector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func linearTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(handler)
	t.Cleanup(s.Close)
	return s
}

func gqlResponse(data map[string]any) map[string]any {
	return map[string]any{"data": data}
}

func TestLinearCreateIssueConnector_CreatesIssue(t *testing.T) {
	var gotAuth string
	var gotVars map[string]any

	server := linearTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		var req struct {
			Variables map[string]any `json:"variables"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		gotVars = req.Variables

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(gqlResponse(map[string]any{
			"issueCreate": map[string]any{
				"success": true,
				"issue": map[string]any{
					"id":         "issue-uuid-1",
					"identifier": "ENG-42",
					"title":      "Fix login bug",
					"url":        "https://linear.app/team/issue/ENG-42",
				},
			},
		}))
	})

	c := &LinearCreateIssueConnector{baseURL: server.URL}
	out, err := c.Execute(context.Background(), map[string]any{
		"team_id":     "team-uuid",
		"title":       "Fix login bug",
		"description": "Users cannot log in on Safari.",
		"priority":    float64(2),
		"_credential": map[string]string{"token": "lin_api_test"},
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if gotAuth != "lin_api_test" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "lin_api_test")
	}
	input, ok := gotVars["input"].(map[string]any)
	if !ok {
		t.Fatalf("variables.input missing or wrong type: %T", gotVars["input"])
	}
	if input["teamId"] != "team-uuid" {
		t.Errorf("input.teamId = %q, want team-uuid", input["teamId"])
	}
	if input["title"] != "Fix login bug" {
		t.Errorf("input.title = %q", input["title"])
	}
	if input["priority"] != float64(2) {
		t.Errorf("input.priority = %v, want 2", input["priority"])
	}
	if out["identifier"] != "ENG-42" {
		t.Errorf("identifier = %q, want ENG-42", out["identifier"])
	}
	if out["url"] != "https://linear.app/team/issue/ENG-42" {
		t.Errorf("url = %q", out["url"])
	}
}

func TestLinearCreateIssueConnector_MissingTeamID(t *testing.T) {
	c := &LinearCreateIssueConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"title":       "Test",
		"_credential": map[string]string{"token": "lin_api_test"},
	})
	if err == nil {
		t.Fatal("expected error for missing team_id")
	}
}

func TestLinearCreateIssueConnector_MissingTitle(t *testing.T) {
	c := &LinearCreateIssueConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"team_id":     "team-uuid",
		"_credential": map[string]string{"token": "lin_api_test"},
	})
	if err == nil {
		t.Fatal("expected error for missing title")
	}
}

func TestLinearCreateIssueConnector_MissingCredential(t *testing.T) {
	c := &LinearCreateIssueConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"team_id": "team-uuid",
		"title":   "Test",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestLinearCreateIssueConnector_GraphQLError(t *testing.T) {
	server := linearTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"errors": []map[string]any{{"message": "Entity not found"}},
		})
	})

	c := &LinearCreateIssueConnector{baseURL: server.URL}
	_, err := c.Execute(context.Background(), map[string]any{
		"team_id":     "bad-team",
		"title":       "Test",
		"_credential": map[string]string{"token": "lin_api_test"},
	})
	if err == nil {
		t.Fatal("expected error for GraphQL error response")
	}
	if got := err.Error(); got == "" {
		t.Errorf("error message is empty")
	}
}

func TestLinearCreateIssueConnector_SuccessFalse(t *testing.T) {
	server := linearTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(gqlResponse(map[string]any{
			"issueCreate": map[string]any{"success": false},
		}))
	})

	c := &LinearCreateIssueConnector{baseURL: server.URL}
	_, err := c.Execute(context.Background(), map[string]any{
		"team_id":     "team-uuid",
		"title":       "Test",
		"_credential": map[string]string{"token": "lin_api_test"},
	})
	if err == nil {
		t.Fatal("expected error when success=false")
	}
}

func TestLinearCreateIssueConnector_CredentialDeleted(t *testing.T) {
	server := linearTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(gqlResponse(map[string]any{
			"issueCreate": map[string]any{
				"success": true,
				"issue": map[string]any{
					"id": "i1", "identifier": "ENG-1", "title": "T",
					"url": "https://linear.app/t/issue/ENG-1",
				},
			},
		}))
	})

	params := map[string]any{
		"team_id":     "team-uuid",
		"title":       "T",
		"_credential": map[string]string{"token": "lin_api_test"},
	}
	c := &LinearCreateIssueConnector{baseURL: server.URL}
	if _, err := c.Execute(context.Background(), params); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if _, exists := params["_credential"]; exists {
		t.Error("_credential should be deleted from params after execution")
	}
}

func TestLinearSearchConnector_ReturnsIssues(t *testing.T) {
	var gotVars map[string]any

	server := linearTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Variables map[string]any `json:"variables"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		gotVars = req.Variables

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(gqlResponse(map[string]any{
			"issues": map[string]any{
				"nodes": []any{
					map[string]any{
						"id": "i1", "identifier": "ENG-1", "title": "Alpha",
						"url": "https://linear.app/t/issue/ENG-1", "priority": float64(1),
						"state":    map[string]any{"name": "In Progress"},
						"assignee": map[string]any{"name": "Alice", "email": "alice@example.com"},
					},
					map[string]any{
						"id": "i2", "identifier": "ENG-2", "title": "Beta",
						"url": "https://linear.app/t/issue/ENG-2", "priority": float64(3),
						"state":    map[string]any{"name": "Todo"},
						"assignee": nil,
					},
				},
			},
		}))
	})

	c := &LinearSearchConnector{baseURL: server.URL}
	out, err := c.Execute(context.Background(), map[string]any{
		"query":       "login",
		"team_id":     "team-uuid",
		"limit":       float64(10),
		"_credential": map[string]string{"token": "lin_api_test"},
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if gotVars["first"] != float64(10) {
		t.Errorf("first = %v, want 10", gotVars["first"])
	}
	filter, ok := gotVars["filter"].(map[string]any)
	if !ok {
		t.Fatalf("filter missing or wrong type: %T", gotVars["filter"])
	}
	if _, hasTitle := filter["title"]; !hasTitle {
		t.Error("filter should contain title for query param")
	}
	if _, hasTeam := filter["team"]; !hasTeam {
		t.Error("filter should contain team for team_id param")
	}

	count, _ := out["count"].(int)
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	issues, _ := out["issues"].([]any)
	if len(issues) != 2 {
		t.Errorf("len(issues) = %d, want 2", len(issues))
	}
}

func TestLinearSearchConnector_DefaultLimit(t *testing.T) {
	var gotFirst any

	server := linearTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Variables map[string]any `json:"variables"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		gotFirst = req.Variables["first"]
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(gqlResponse(map[string]any{
			"issues": map[string]any{"nodes": []any{}},
		}))
	})

	c := &LinearSearchConnector{baseURL: server.URL}
	_, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{"token": "lin_api_test"},
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if gotFirst != float64(25) {
		t.Errorf("default limit = %v, want 25", gotFirst)
	}
}

func TestLinearSearchConnector_NoFilter(t *testing.T) {
	var gotVars map[string]any

	server := linearTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Variables map[string]any `json:"variables"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		gotVars = req.Variables
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(gqlResponse(map[string]any{
			"issues": map[string]any{"nodes": []any{}},
		}))
	})

	c := &LinearSearchConnector{baseURL: server.URL}
	_, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{"token": "lin_api_test"},
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if _, hasFilter := gotVars["filter"]; hasFilter {
		t.Error("filter should be absent when no filter params provided")
	}
}

func TestLinearSearchConnector_MissingCredential(t *testing.T) {
	c := &LinearSearchConnector{}
	_, err := c.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestRegistry_LinearConnectors(t *testing.T) {
	r := NewRegistry()
	for _, action := range []string{"linear/create_issue", "linear/search"} {
		if _, err := r.Get(action); err != nil {
			t.Errorf("Get(%q) error: %v", action, err)
		}
	}
}
