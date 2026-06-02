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

func TestBuildDeclarativeScript_FreshSession(t *testing.T) {
	script := buildDeclarativeScript("actionData.x = 1;", nil, 30000, false)
	for _, want := range []string{
		"chromium.launch()",
		"actionData.x = 1;",
		"setDefaultTimeout(30000)",
		"session_state",
		"JSON.stringify",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("script missing %q", want)
		}
	}
	if strings.Contains(script, "addCookies") {
		t.Error("fresh session should not call addCookies")
	}
	if strings.Contains(script, "page.goto") {
		t.Error("fresh session should not restore URL")
	}
}

func TestBuildDeclarativeScript_WithSession_RestoresURL(t *testing.T) {
	session := &BrowserSession{
		Cookies: []map[string]any{
			{"name": "sid", "value": "abc", "domain": "example.com", "path": "/"},
		},
		LocalStorage: map[string]map[string]string{},
		URL:          "https://example.com/dashboard",
	}
	script := buildDeclarativeScript("", session, 5000, false)
	if !strings.Contains(script, "addCookies") {
		t.Error("should restore cookies")
	}
	if !strings.Contains(script, "example.com/dashboard") {
		t.Error("should restore URL")
	}
	if !strings.Contains(script, "setDefaultTimeout(5000)") {
		t.Error("should use provided timeout")
	}
}

func TestBuildDeclarativeScript_SkipURLRestore(t *testing.T) {
	session := &BrowserSession{
		Cookies: []map[string]any{},
		URL:     "https://old.example.com/page",
	}
	script := buildDeclarativeScript("await page.goto('https://new.example.com');", session, 30000, true)
	if strings.Contains(script, "old.example.com") {
		t.Error("skipURLRestore=true must not navigate to previous URL")
	}
	if !strings.Contains(script, "new.example.com") {
		t.Error("action snippet must be present")
	}
}
