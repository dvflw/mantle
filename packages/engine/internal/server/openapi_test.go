package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleOpenAPISpec(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.json", nil)
	handleOpenAPISpec(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var doc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	// Swagger 2.0 uses "swagger", OpenAPI 3.x uses "openapi"
	if doc["swagger"] == nil && doc["openapi"] == nil {
		t.Error("response does not look like an OpenAPI/Swagger document (missing 'swagger' or 'openapi' key)")
	}
	if doc["info"] == nil {
		t.Error("response missing 'info' field")
	}
	if doc["paths"] == nil {
		t.Error("response missing 'paths' field")
	}
}
