package server

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestNormalizePath_CollapseUUID(t *testing.T) {
	input := "/api/v1/executions/550e8400-e29b-41d4-a716-446655440000/steps/f47ac10b-58cc-4372-a567-0e02b2c3d479"
	got := normalizePath(input)
	want := "/api/v1/executions/{id}/steps/{id}"
	if got != want {
		t.Errorf("normalizePath(%q) = %q, want %q", input, got, want)
	}
}

func TestNormalizePath_PreservesStatic(t *testing.T) {
	paths := []string{
		"/healthz",
		"/api/v1/workflows",
		"/hooks/my-webhook-path",
	}
	for _, p := range paths {
		got := normalizePath(p)
		if got != p {
			t.Errorf("normalizePath(%q) = %q, want unchanged", p, got)
		}
	}
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, 200, map[string]string{"hello": "world"})

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if rec.Code != 200 {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if body["hello"] != "world" {
		t.Errorf("body[hello] = %q, want world", body["hello"])
	}
}

func TestWriteJSONError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSONError(rec, "something went wrong", 422)

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if rec.Code != 422 {
		t.Errorf("status = %d, want 422", rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if body["error"] != "something went wrong" {
		t.Errorf("body[error] = %q, want 'something went wrong'", body["error"])
	}
}
