package connector

import (
	"strings"
	"testing"
)

func TestExtractSession_Absent(t *testing.T) {
	s, err := extractSession(map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s != nil {
		t.Fatal("expected nil session for absent session_state")
	}
}

func TestExtractSession_Nil(t *testing.T) {
	s, err := extractSession(map[string]any{"session_state": nil})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s != nil {
		t.Fatal("expected nil session for nil session_state")
	}
}

func TestExtractSession_Valid(t *testing.T) {
	params := map[string]any{
		"session_state": map[string]any{
			"cookies":       []any{},
			"local_storage": map[string]any{},
			"url":           "https://example.com/dashboard",
		},
	}
	s, err := extractSession(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s == nil {
		t.Fatal("expected non-nil session")
	}
	if s.URL != "https://example.com/dashboard" {
		t.Errorf("URL = %q, want %q", s.URL, "https://example.com/dashboard")
	}
}

func TestExtractSession_Invalid(t *testing.T) {
	_, err := extractSession(map[string]any{"session_state": "not-an-object"})
	if err == nil {
		t.Fatal("expected error for invalid session_state")
	}
	if !strings.Contains(err.Error(), "invalid session_state") {
		t.Errorf("error = %q, want 'invalid session_state'", err)
	}
}

func TestExtractTimeoutMs_Default(t *testing.T) {
	if got := extractTimeoutMs(map[string]any{}); got != 30000 {
		t.Errorf("got %d, want 30000", got)
	}
}

func TestExtractTimeoutMs_Provided(t *testing.T) {
	if got := extractTimeoutMs(map[string]any{"timeout_ms": float64(5000)}); got != 5000 {
		t.Errorf("got %d, want 5000", got)
	}
}
