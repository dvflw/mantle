package connector

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func notionPageResponse() map[string]any {
	return map[string]any{
		"id":               "page-uuid-1",
		"url":              "https://www.notion.so/Test-Page-page-uuid-1",
		"created_time":     "2024-01-01T00:00:00.000Z",
		"last_edited_time": "2024-01-01T00:00:00.000Z",
	}
}

func TestNotionCreatePageConnector_CreatesPageInDatabase(t *testing.T) {
	var gotAuth, gotVersion string
	var gotBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotVersion = r.Header.Get("Notion-Version")
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(notionPageResponse())
	}))
	defer server.Close()

	c := &NotionCreatePageConnector{baseURL: server.URL}
	out, err := c.Execute(context.Background(), map[string]any{
		"parent_database_id": "db-uuid-1",
		"title":              "Test Page",
		"_credential":        map[string]string{"token": "secret_test"},
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if gotAuth != "Bearer secret_test" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer secret_test")
	}
	if gotVersion != notionVersion {
		t.Errorf("Notion-Version = %q, want %q", gotVersion, notionVersion)
	}
	parent, _ := gotBody["parent"].(map[string]any)
	if parent["database_id"] != "db-uuid-1" {
		t.Errorf("parent.database_id = %q, want db-uuid-1", parent["database_id"])
	}
	if out["id"] != "page-uuid-1" {
		t.Errorf("id = %q, want page-uuid-1", out["id"])
	}
	if out["url"] != "https://www.notion.so/Test-Page-page-uuid-1" {
		t.Errorf("url = %q", out["url"])
	}
}

func TestNotionCreatePageConnector_CreatesPageUnderPage(t *testing.T) {
	var gotParent map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		gotParent, _ = body["parent"].(map[string]any)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(notionPageResponse())
	}))
	defer server.Close()

	c := &NotionCreatePageConnector{baseURL: server.URL}
	if _, err := c.Execute(context.Background(), map[string]any{
		"parent_page_id": "parent-page-uuid",
		"_credential":    map[string]string{"token": "secret_test"},
	}); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if gotParent["page_id"] != "parent-page-uuid" {
		t.Errorf("parent.page_id = %q, want parent-page-uuid", gotParent["page_id"])
	}
	if _, hasDatabaseID := gotParent["database_id"]; hasDatabaseID {
		t.Error("parent should not contain database_id for page parent")
	}
}

func TestNotionCreatePageConnector_TitleSetInProperties(t *testing.T) {
	var gotBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(notionPageResponse())
	}))
	defer server.Close()

	c := &NotionCreatePageConnector{baseURL: server.URL}
	if _, err := c.Execute(context.Background(), map[string]any{
		"parent_database_id": "db-uuid-1",
		"title":              "My Page",
		"_credential":        map[string]string{"token": "secret_test"},
	}); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	props, _ := gotBody["properties"].(map[string]any)
	titleProp, ok := props["title"].(map[string]any)
	if !ok {
		t.Fatal("properties.title missing or wrong type")
	}
	titleArr, _ := titleProp["title"].([]any)
	if len(titleArr) == 0 {
		t.Fatal("properties.title.title is empty")
	}
	text, _ := titleArr[0].(map[string]any)
	textContent, _ := text["text"].(map[string]any)
	if textContent["content"] != "My Page" {
		t.Errorf("title content = %q, want My Page", textContent["content"])
	}
}

func TestNotionCreatePageConnector_ExplicitPropertiesNotOverriddenByTitle(t *testing.T) {
	var gotBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(notionPageResponse())
	}))
	defer server.Close()

	customTitle := map[string]any{
		"title": []any{map[string]any{"text": map[string]any{"content": "Explicit Title"}}},
	}
	c := &NotionCreatePageConnector{baseURL: server.URL}
	if _, err := c.Execute(context.Background(), map[string]any{
		"parent_database_id": "db-uuid-1",
		"title":              "Ignored Title",
		"properties":         map[string]any{"title": customTitle},
		"_credential":        map[string]string{"token": "secret_test"},
	}); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	props, _ := gotBody["properties"].(map[string]any)
	titleProp, _ := props["title"].(map[string]any)
	titleArr, _ := titleProp["title"].([]any)
	text, _ := titleArr[0].(map[string]any)
	textContent, _ := text["text"].(map[string]any)
	if textContent["content"] != "Explicit Title" {
		t.Errorf("explicit properties.title was overridden by title shorthand: content = %q", textContent["content"])
	}
}

func TestNotionCreatePageConnector_CustomTitleKey(t *testing.T) {
	var gotBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(notionPageResponse())
	}))
	defer server.Close()

	c := &NotionCreatePageConnector{baseURL: server.URL}
	if _, err := c.Execute(context.Background(), map[string]any{
		"parent_database_id": "db-uuid-1",
		"title":              "My Task",
		"title_key":          "Name",
		"_credential":        map[string]string{"token": "secret_test"},
	}); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	props, _ := gotBody["properties"].(map[string]any)
	if _, hasTitle := props["title"]; hasTitle {
		t.Error("properties should not contain key 'title' when title_key=Name")
	}
	nameProp, ok := props["Name"].(map[string]any)
	if !ok {
		t.Fatal("properties.Name missing or wrong type")
	}
	titleArr, _ := nameProp["title"].([]any)
	text, _ := titleArr[0].(map[string]any)
	textContent, _ := text["text"].(map[string]any)
	if textContent["content"] != "My Task" {
		t.Errorf("Name content = %q, want My Task", textContent["content"])
	}
}

func TestNotionCreatePageConnector_MissingParent(t *testing.T) {
	c := &NotionCreatePageConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"title":       "Test",
		"_credential": map[string]string{"token": "secret_test"},
	})
	if err == nil {
		t.Fatal("expected error for missing parent")
	}
}

func TestNotionCreatePageConnector_MissingCredential(t *testing.T) {
	c := &NotionCreatePageConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"parent_database_id": "db-uuid-1",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestNotionCreatePageConnector_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"object":"error","status":401,"message":"API token is invalid."}`))
	}))
	defer server.Close()

	c := &NotionCreatePageConnector{baseURL: server.URL}
	_, err := c.Execute(context.Background(), map[string]any{
		"parent_database_id": "db-uuid-1",
		"title":              "Test",
		"_credential":        map[string]string{"token": "bad_token"},
	})
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}

func TestNotionCreatePageConnector_CredentialDeleted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(notionPageResponse())
	}))
	defer server.Close()

	params := map[string]any{
		"parent_database_id": "db-uuid-1",
		"title":              "Test",
		"_credential":        map[string]string{"token": "secret_test"},
	}
	c := &NotionCreatePageConnector{baseURL: server.URL}
	if _, err := c.Execute(context.Background(), params); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if _, exists := params["_credential"]; exists {
		t.Error("_credential should be deleted from params after execution")
	}
}

func TestNotionQueryDatabaseConnector_ReturnsResults(t *testing.T) {
	var gotBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		if r.URL.Path != "/databases/db-uuid-1/query" {
			t.Errorf("path = %q, want /databases/db-uuid-1/query", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"results": []any{
				map[string]any{"id": "page-1", "object": "page"},
				map[string]any{"id": "page-2", "object": "page"},
			},
			"next_cursor": nil,
			"has_more":    false,
		})
	}))
	defer server.Close()

	c := &NotionQueryDatabaseConnector{baseURL: server.URL}
	out, err := c.Execute(context.Background(), map[string]any{
		"database_id": "db-uuid-1",
		"page_size":   float64(10),
		"_credential": map[string]string{"token": "secret_test"},
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if gotBody["page_size"] != float64(10) {
		t.Errorf("page_size = %v, want 10", gotBody["page_size"])
	}
	count, _ := out["count"].(int)
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	results, _ := out["results"].([]any)
	if len(results) != 2 {
		t.Errorf("len(results) = %d, want 2", len(results))
	}
	if out["has_more"] != false {
		t.Errorf("has_more = %v, want false", out["has_more"])
	}
}

func TestNotionQueryDatabaseConnector_WithFilterAndSorts(t *testing.T) {
	var gotBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"results": []any{}, "has_more": false,
		})
	}))
	defer server.Close()

	filter := map[string]any{
		"property": "Status",
		"select":   map[string]any{"equals": "Done"},
	}
	sorts := []any{
		map[string]any{"property": "Created", "direction": "descending"},
	}

	c := &NotionQueryDatabaseConnector{baseURL: server.URL}
	if _, err := c.Execute(context.Background(), map[string]any{
		"database_id": "db-uuid-1",
		"filter":      filter,
		"sorts":       sorts,
		"_credential": map[string]string{"token": "secret_test"},
	}); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if _, ok := gotBody["filter"]; !ok {
		t.Error("expected filter in request body")
	}
	if _, ok := gotBody["sorts"]; !ok {
		t.Error("expected sorts in request body")
	}
}

func TestNotionQueryDatabaseConnector_HasMore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"results":     []any{map[string]any{"id": "page-1"}},
			"next_cursor": "cursor-abc",
			"has_more":    true,
		})
	}))
	defer server.Close()

	c := &NotionQueryDatabaseConnector{baseURL: server.URL}
	out, err := c.Execute(context.Background(), map[string]any{
		"database_id": "db-uuid-1",
		"_credential": map[string]string{"token": "secret_test"},
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if out["has_more"] != true {
		t.Errorf("has_more = %v, want true", out["has_more"])
	}
	if out["next_cursor"] != "cursor-abc" {
		t.Errorf("next_cursor = %q, want cursor-abc", out["next_cursor"])
	}
}

func TestNotionQueryDatabaseConnector_NoNextCursorWhenEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"results": []any{}, "next_cursor": "", "has_more": false,
		})
	}))
	defer server.Close()

	c := &NotionQueryDatabaseConnector{baseURL: server.URL}
	out, err := c.Execute(context.Background(), map[string]any{
		"database_id": "db-uuid-1",
		"_credential": map[string]string{"token": "secret_test"},
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if _, exists := out["next_cursor"]; exists {
		t.Error("next_cursor should be absent when empty")
	}
}

func TestNotionCreatePageConnector_MapAnyCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(notionPageResponse())
	}))
	defer server.Close()

	c := &NotionCreatePageConnector{baseURL: server.URL}
	// Simulate a credential delivered as map[string]any (JSON/CEL-deserialised shape).
	_, err := c.Execute(context.Background(), map[string]any{
		"parent_database_id": "db-uuid-1",
		"title":              "Test",
		"_credential":        map[string]any{"token": "secret_test"},
	})
	if err != nil {
		t.Fatalf("Execute() with map[string]any credential: %v", err)
	}
}

func TestNotionQueryDatabaseConnector_MissingDatabaseID(t *testing.T) {
	c := &NotionQueryDatabaseConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"_credential": map[string]string{"token": "secret_test"},
	})
	if err == nil {
		t.Fatal("expected error for missing database_id")
	}
}

func TestNotionQueryDatabaseConnector_MissingCredential(t *testing.T) {
	c := &NotionQueryDatabaseConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"database_id": "db-uuid-1",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestNotionQueryDatabaseConnector_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"object":"error","status":404,"message":"Could not find database."}`))
	}))
	defer server.Close()

	c := &NotionQueryDatabaseConnector{baseURL: server.URL}
	_, err := c.Execute(context.Background(), map[string]any{
		"database_id": "nonexistent",
		"_credential": map[string]string{"token": "secret_test"},
	})
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}

func TestRegistry_NotionConnectors(t *testing.T) {
	r := NewRegistry()
	for _, action := range []string{"notion/create_page", "notion/query_database"} {
		if _, err := r.Get(action); err != nil {
			t.Errorf("Get(%q) error: %v", action, err)
		}
	}
}
