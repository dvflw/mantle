package connector

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOneDriveUploadConnector_UploadsFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		if r.URL.Path != "/me/drive/root:/documents/hello.txt:/content" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer graph-token" {
			t.Errorf("unexpected Authorization: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"id":   "file-id-1",
			"name": "hello.txt",
			"size": 13,
		})
	}))
	defer srv.Close()

	c := &OneDriveUploadConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "graph-token"},
		"path":        "documents/hello.txt",
		"content":     "Hello, Mantle!",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["id"] != "file-id-1" {
		t.Errorf("expected id=file-id-1, got %v", out["id"])
	}
}

func TestOneDriveUploadConnector_MissingPath(t *testing.T) {
	c := &OneDriveUploadConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "tok"},
		"content":     "data",
	})
	if err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestOneDriveUploadConnector_MissingContent(t *testing.T) {
	c := &OneDriveUploadConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "tok"},
		"path":        "file.txt",
	})
	if err == nil {
		t.Fatal("expected error for missing content")
	}
}

func TestOneDriveUploadConnector_MissingCredential(t *testing.T) {
	c := &OneDriveUploadConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"path":    "file.txt",
		"content": "data",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestOneDriveUploadConnector_WithDriveID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/drives/drive123/root:/notes.txt:/content" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"id": "f2", "name": "notes.txt"})
	}))
	defer srv.Close()

	c := &OneDriveUploadConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "tok"},
		"path":        "notes.txt",
		"content":     "note content",
		"drive_id":    "drive123",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSharePointListItemsConnector_ListsItems(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer sp-token" {
			t.Errorf("unexpected Authorization: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"value": []any{
				map[string]any{"id": "1", "fields": map[string]any{"Title": "Row 1"}},
				map[string]any{"id": "2", "fields": map[string]any{"Title": "Row 2"}},
			},
		})
	}))
	defer srv.Close()

	c := &SharePointListItemsConnector{baseURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "sp-token"},
		"site_id":     "site1",
		"list_id":     "list1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["count"].(int) != 2 {
		t.Errorf("expected count=2, got %v", out["count"])
	}
}

func TestSharePointListItemsConnector_MissingSiteID(t *testing.T) {
	c := &SharePointListItemsConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "tok"},
		"list_id":     "list1",
	})
	if err == nil {
		t.Fatal("expected error for missing site_id")
	}
}

func TestSharePointListItemsConnector_MissingListID(t *testing.T) {
	c := &SharePointListItemsConnector{baseURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "tok"},
		"site_id":     "site1",
	})
	if err == nil {
		t.Fatal("expected error for missing list_id")
	}
}

func TestOneDriveUploadConnector_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"code":"AccessDenied"}}`, http.StatusForbidden)
	}))
	defer srv.Close()

	c := &OneDriveUploadConnector{baseURL: srv.URL}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "bad"},
		"path":        "file.txt",
		"content":     "data",
	})
	if err == nil {
		t.Fatal("expected error for 403")
	}
}

func TestRegistry_OneDriveSharePointConnectors(t *testing.T) {
	r := NewRegistry()
	for _, action := range []string{"onedrive/upload", "sharepoint/list_items"} {
		if _, err := r.Get(action); err != nil {
			t.Errorf("%s not registered: %v", action, err)
		}
	}
}
