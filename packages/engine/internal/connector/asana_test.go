package connector

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAsanaCreateTaskConnector_CreatesTask(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/tasks" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer asana-token" {
			t.Errorf("unexpected Authorization: %s", r.Header.Get("Authorization"))
		}

		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		data := body["data"].(map[string]any)
		if data["name"] != "Fix the bug" {
			t.Errorf("unexpected name: %v", data["name"])
		}
		if data["workspace"] != "12345" {
			t.Errorf("unexpected workspace: %v", data["workspace"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"gid":           "67890",
				"name":          "Fix the bug",
				"permalink_url": "https://app.asana.com/0/0/67890",
			},
		})
	}))
	defer srv.Close()

	c := &AsanaCreateTaskConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "asana-token"},
		"name":        "Fix the bug",
		"workspace":   "12345",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["gid"] != "67890" {
		t.Errorf("expected gid=67890, got %v", out["gid"])
	}
	if out["permalink_url"] != "https://app.asana.com/0/0/67890" {
		t.Errorf("unexpected permalink_url: %v", out["permalink_url"])
	}
}

func TestAsanaCreateTaskConnector_WithOptionalFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		data := body["data"].(map[string]any)
		if data["notes"] != "Details here" {
			t.Errorf("unexpected notes: %v", data["notes"])
		}
		if data["assignee"] != "me" {
			t.Errorf("unexpected assignee: %v", data["assignee"])
		}
		if data["due_on"] != "2024-12-31" {
			t.Errorf("unexpected due_on: %v", data["due_on"])
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"gid": "1", "name": "t", "permalink_url": ""},
		})
	}))
	defer srv.Close()

	c := &AsanaCreateTaskConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "asana-token"},
		"name":        "t",
		"workspace":   "12345",
		"notes":       "Details here",
		"assignee":    "me",
		"due_on":      "2024-12-31",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAsanaCreateTaskConnector_WithProjects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		data := body["data"].(map[string]any)
		projects, _ := data["projects"].([]any)
		if len(projects) != 1 || projects[0] != "PROJ123" {
			t.Errorf("unexpected projects: %v", projects)
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"gid": "1", "name": "t", "permalink_url": ""},
		})
	}))
	defer srv.Close()

	c := &AsanaCreateTaskConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "asana-token"},
		"name":        "t",
		"projects":    []any{"PROJ123"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAsanaCreateTaskConnector_MissingName(t *testing.T) {
	c := &AsanaCreateTaskConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "asana-token"},
		"workspace":   "12345",
	})
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestAsanaCreateTaskConnector_MissingWorkspaceAndProjects(t *testing.T) {
	c := &AsanaCreateTaskConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "asana-token"},
		"name":        "t",
	})
	if err == nil {
		t.Fatal("expected error when neither workspace nor projects provided")
	}
}

func TestAsanaCreateTaskConnector_MissingCredential(t *testing.T) {
	c := &AsanaCreateTaskConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"name":      "t",
		"workspace": "12345",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestAsanaCreateTaskConnector_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"errors":[{"message":"Invalid token"}]}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := &AsanaCreateTaskConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "bad"},
		"name":        "t",
		"workspace":   "12345",
	})
	if err == nil {
		t.Fatal("expected error for 401")
	}
}

func TestAsanaSearchConnector_SearchesTasks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/workspaces/WS123/tasks/search" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("text") != "bug" {
			t.Errorf("unexpected text param: %s", r.URL.Query().Get("text"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": []any{
				map[string]any{"gid": "1", "name": "Bug fix"},
				map[string]any{"gid": "2", "name": "Bug report"},
			},
		})
	}))
	defer srv.Close()

	c := &AsanaSearchConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "asana-token"},
		"workspace":   "WS123",
		"text":        "bug",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["count"].(int) != 2 {
		t.Errorf("expected count=2, got %v", out["count"])
	}
}

func TestAsanaSearchConnector_WithFilters(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("assignee.any") != "me" {
			t.Errorf("unexpected assignee.any: %s", q.Get("assignee.any"))
		}
		if q.Get("completed") != "false" {
			t.Errorf("unexpected completed: %s", q.Get("completed"))
		}
		if q.Get("limit") != "20" {
			t.Errorf("unexpected limit: %s", q.Get("limit"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
	}))
	defer srv.Close()

	c := &AsanaSearchConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "asana-token"},
		"workspace":   "WS123",
		"assignee":    "me",
		"completed":   false,
		"limit":       20,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAsanaSearchConnector_MissingWorkspace(t *testing.T) {
	c := &AsanaSearchConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "asana-token"},
		"text":        "bug",
	})
	if err == nil {
		t.Fatal("expected error for missing workspace")
	}
}

func TestAsanaSearchConnector_MissingCredential(t *testing.T) {
	c := &AsanaSearchConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"workspace": "WS123",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestAsanaSearchConnector_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"errors":[{"message":"Forbidden"}]}`, http.StatusForbidden)
	}))
	defer srv.Close()

	c := &AsanaSearchConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "asana-token"},
		"workspace":   "WS123",
	})
	if err == nil {
		t.Fatal("expected error for 403")
	}
}

func TestAsanaCreateTaskConnector_MapAnyCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer mapany" {
			t.Errorf("unexpected auth: %s", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"gid": "1", "name": "t", "permalink_url": ""},
		})
	}))
	defer srv.Close()

	c := &AsanaCreateTaskConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]any{"token": "mapany"},
		"name":        "t",
		"workspace":   "12345",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRegistry_AsanaConnectors(t *testing.T) {
	r := NewRegistry()
	if _, err := r.Get("asana/create_task"); err != nil {
		t.Errorf("asana/create_task not registered: %v", err)
	}
	if _, err := r.Get("asana/search"); err != nil {
		t.Errorf("asana/search not registered: %v", err)
	}
}
