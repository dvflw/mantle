package connector

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestElasticsearchSearchConnector_SearchesDocuments(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/my-index/_search" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != "elastic" || pass != "secret" {
			t.Errorf("unexpected basic auth: %s/%s", user, pass)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"took": 5,
			"hits": map[string]any{
				"total": map[string]any{"value": 2},
				"hits":  []any{map[string]any{"_id": "1"}, map[string]any{"_id": "2"}},
			},
		})
	}))
	defer srv.Close()

	c := &ElasticsearchSearchConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"url": srv.URL, "username": "elastic", "password": "secret"},
		"index":       "my-index",
		"query":       map[string]any{"match_all": map[string]any{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["total"].(int) != 2 {
		t.Errorf("expected total=2, got %v", out["total"])
	}
	if out["count"].(int) != 2 {
		t.Errorf("expected count=2, got %v", out["count"])
	}
}

func TestElasticsearchSearchConnector_WithAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "ApiKey myapikey" {
			t.Errorf("unexpected Authorization: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"took": 1,
			"hits": map[string]any{
				"total": map[string]any{"value": 0},
				"hits":  []any{},
			},
		})
	}))
	defer srv.Close()

	c := &ElasticsearchSearchConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"url": srv.URL, "api_key": "myapikey"},
		"index":       "my-index",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestElasticsearchSearchConnector_MissingIndex(t *testing.T) {
	c := &ElasticsearchSearchConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"url": "http://unused"},
	})
	if err == nil {
		t.Fatal("expected error for missing index")
	}
}

func TestElasticsearchSearchConnector_MissingCredential(t *testing.T) {
	c := &ElasticsearchSearchConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"index": "my-index",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestElasticsearchSearchConnector_MissingURL(t *testing.T) {
	c := &ElasticsearchSearchConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"username": "elastic"},
		"index":       "my-index",
	})
	if err == nil {
		t.Fatal("expected error for missing url in credential")
	}
}

func TestElasticsearchSearchConnector_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"index_not_found"}`, http.StatusNotFound)
	}))
	defer srv.Close()

	c := &ElasticsearchSearchConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"url": srv.URL},
		"index":       "missing-index",
	})
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestElasticsearchIndexConnector_IndexesDocument(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/my-index/_doc" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["name"] != "Alice" {
			t.Errorf("unexpected document: %v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"_id":    "abc123",
			"_index": "my-index",
			"result": "created",
		})
	}))
	defer srv.Close()

	c := &ElasticsearchIndexConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"url": srv.URL},
		"index":       "my-index",
		"document":    map[string]any{"name": "Alice"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["id"] != "abc123" {
		t.Errorf("expected id=abc123, got %v", out["id"])
	}
	if out["result"] != "created" {
		t.Errorf("expected result=created, got %v", out["result"])
	}
}

func TestElasticsearchIndexConnector_WithExplicitID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" || r.URL.Path != "/my-index/_doc/myid" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"_id": "myid", "_index": "my-index", "result": "updated"})
	}))
	defer srv.Close()

	c := &ElasticsearchIndexConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"url": srv.URL},
		"index":       "my-index",
		"id":          "myid",
		"document":    map[string]any{"name": "Bob"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestElasticsearchIndexConnector_MissingDocument(t *testing.T) {
	c := &ElasticsearchIndexConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"url": "http://unused"},
		"index":       "my-index",
	})
	if err == nil {
		t.Fatal("expected error for missing document")
	}
}

func TestElasticsearchSearchConnector_MapAnyCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "ApiKey mapkey" {
			t.Errorf("unexpected auth: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"took": 1, "hits": map[string]any{"total": map[string]any{"value": 0}, "hits": []any{}},
		})
	}))
	defer srv.Close()

	c := &ElasticsearchSearchConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]any{"url": srv.URL, "api_key": "mapkey"},
		"index":       "my-index",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRegistry_ElasticsearchConnectors(t *testing.T) {
	r := NewRegistry()
	if _, err := r.Get("elasticsearch/search"); err != nil {
		t.Errorf("elasticsearch/search not registered: %v", err)
	}
	if _, err := r.Get("elasticsearch/index"); err != nil {
		t.Errorf("elasticsearch/index not registered: %v", err)
	}
}
