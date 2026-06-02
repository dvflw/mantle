package connector

import (
	"context"
	"fmt"
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
	script, err := buildDeclarativeScript("actionData.x = 1;", nil, 30000, false)
	if err != nil {
		t.Fatalf("buildDeclarativeScript: %v", err)
	}
	for _, want := range []string{
		"chromium.launch({ headless: true })",
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
	script, err := buildDeclarativeScript("", session, 5000, false)
	if err != nil {
		t.Fatalf("buildDeclarativeScript: %v", err)
	}
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
		Cookies: []map[string]any{
			{"name": "sid", "value": "abc", "domain": "example.com", "path": "/"},
		},
		URL: "https://old.example.com/page",
	}
	script, err := buildDeclarativeScript("await page.goto('https://new.example.com');", session, 30000, true)
	if err != nil {
		t.Fatalf("buildDeclarativeScript: %v", err)
	}
	if strings.Contains(script, "old.example.com") {
		t.Error("skipURLRestore=true must not navigate to previous URL")
	}
	if !strings.Contains(script, "new.example.com") {
		t.Error("action snippet must be present")
	}
	if !strings.Contains(script, "addCookies") {
		t.Error("skipURLRestore=true should still restore cookies")
	}
}

func TestBrowserNavigate_MissingURL(t *testing.T) {
	c := &BrowserNavigateConnector{}
	_, err := c.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing url")
	}
	if !strings.Contains(err.Error(), "url is required") {
		t.Errorf("error = %q, want 'url is required'", err)
	}
}

func TestBrowserNavigate_InvalidWaitUntil(t *testing.T) {
	c := &BrowserNavigateConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"url":        "https://example.com",
		"wait_until": "forever",
	})
	if err == nil {
		t.Fatal("expected error for invalid wait_until")
	}
	if !strings.Contains(err.Error(), "wait_until") {
		t.Errorf("error = %q, want 'wait_until' mention", err)
	}
}

func TestBrowserNavigate_InvalidSession(t *testing.T) {
	c := &BrowserNavigateConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"url":           "https://example.com",
		"session_state": "not-a-map",
	})
	if err == nil {
		t.Fatal("expected error for invalid session_state")
	}
}

func TestBrowserClick_MissingSelector(t *testing.T) {
	c := &BrowserClickConnector{}
	_, err := c.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing selector")
	}
	if !strings.Contains(err.Error(), "selector is required") {
		t.Errorf("error = %q, want 'selector is required'", err)
	}
}

func TestBrowserClick_InvalidSession(t *testing.T) {
	c := &BrowserClickConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"selector":      "#btn",
		"session_state": "bad",
	})
	if err == nil {
		t.Fatal("expected error for invalid session_state")
	}
}

func TestBrowserClick_ScriptContainsSelector(t *testing.T) {
	snippet := fmt.Sprintf("await page.click(%s);\n", mustJSONString("#submit-btn"))
	script, err := buildDeclarativeScript(snippet, nil, 30000, false)
	if err != nil {
		t.Fatalf("buildDeclarativeScript: %v", err)
	}
	if !strings.Contains(script, `page.click("#submit-btn")`) {
		t.Error("script should contain page.click with the selector")
	}
}

func TestBrowserClick_ScriptContainsWaitFor(t *testing.T) {
	snippet := fmt.Sprintf("await page.click(%s);\nawait page.waitForSelector(%s);\n",
		mustJSONString("#btn"), mustJSONString(".result"))
	script, err := buildDeclarativeScript(snippet, nil, 30000, false)
	if err != nil {
		t.Fatalf("buildDeclarativeScript: %v", err)
	}
	if !strings.Contains(script, `waitForSelector(".result")`) {
		t.Error("script should contain waitForSelector")
	}
}
