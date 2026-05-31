package connector

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGoogleDriveUploadConnector_UploadsFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		ct := r.Header.Get("Content-Type")
		if !strings.Contains(ct, "multipart/related") {
			t.Errorf("expected multipart/related Content-Type, got %s", ct)
		}
		if r.Header.Get("Authorization") != "Bearer drive-token" {
			t.Errorf("unexpected Authorization: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"id":       "file-id-123",
			"name":     "report.txt",
			"mimeType": "text/plain",
		})
	}))
	defer srv.Close()

	c := &GoogleDriveUploadConnector{uploadURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "drive-token"},
		"name":        "report.txt",
		"content":     "Hello from Mantle",
		"mime_type":   "text/plain",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["id"] != "file-id-123" {
		t.Errorf("expected id=file-id-123, got %v", out["id"])
	}
	if out["name"] != "report.txt" {
		t.Errorf("expected name=report.txt, got %v", out["name"])
	}
}

func TestGoogleDriveUploadConnector_MissingName(t *testing.T) {
	c := &GoogleDriveUploadConnector{uploadURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "tok"},
		"content":     "data",
	})
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestGoogleDriveUploadConnector_MissingContent(t *testing.T) {
	c := &GoogleDriveUploadConnector{uploadURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "tok"},
		"name":        "file.txt",
	})
	if err == nil {
		t.Fatal("expected error for missing content")
	}
}

func TestGoogleDriveUploadConnector_MissingCredential(t *testing.T) {
	c := &GoogleDriveUploadConnector{uploadURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"name":    "file.txt",
		"content": "data",
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestGoogleDriveListFilesConnector_ListsFiles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer list-token" {
			t.Errorf("unexpected Authorization: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"files": []any{
				map[string]any{"id": "f1", "name": "doc1.txt", "mimeType": "text/plain"},
				map[string]any{"id": "f2", "name": "doc2.txt", "mimeType": "text/plain"},
			},
		})
	}))
	defer srv.Close()

	c := &GoogleDriveListFilesConnector{filesURL: srv.URL}
	out, err := c.Execute(t.Context(), map[string]any{
		"_credential": map[string]string{"token": "list-token"},
		"page_size":   10,
	})
	if err != nil {
		t.Fatal(err)
	}
	files, _ := out["files"].([]any)
	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d", len(files))
	}
	if out["count"].(int) != 2 {
		t.Errorf("expected count=2, got %v", out["count"])
	}
}

func TestGoogleDriveListFilesConnector_MissingCredential(t *testing.T) {
	c := &GoogleDriveListFilesConnector{filesURL: "http://unused"}
	_, err := c.Execute(t.Context(), map[string]any{
		"page_size": 10,
	})
	if err == nil {
		t.Fatal("expected error for missing credential")
	}
}

func TestRegistry_GoogleDriveConnectors(t *testing.T) {
	r := NewRegistry()
	for _, action := range []string{"drive/upload", "drive/list_files"} {
		if _, err := r.Get(action); err != nil {
			t.Errorf("%s not registered: %v", action, err)
		}
	}
}
