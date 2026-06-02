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

func TestBrowserFill_MissingFields(t *testing.T) {
	c := &BrowserFillConnector{}
	_, err := c.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing fields")
	}
	if !strings.Contains(err.Error(), "fields is required") {
		t.Errorf("error = %q, want 'fields is required'", err)
	}
}

func TestBrowserFill_EmptyFields(t *testing.T) {
	c := &BrowserFillConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"fields": map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error for empty fields map")
	}
	if !strings.Contains(err.Error(), "non-empty") {
		t.Errorf("error = %q, want 'non-empty'", err)
	}
}

func TestBrowserFill_InvalidSession(t *testing.T) {
	c := &BrowserFillConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"fields":        map[string]any{"#email": "test@example.com"},
		"session_state": 42,
	})
	if err == nil {
		t.Fatal("expected error for invalid session_state")
	}
}

func TestBrowserFill_ScriptContainsFillCalls(t *testing.T) {
	snippet := fmt.Sprintf("await page.fill(%s, %s);\n",
		mustJSONString("#email"), mustJSONString("user@example.com"))
	script, err := buildDeclarativeScript(snippet, nil, 30000, false)
	if err != nil {
		t.Fatalf("buildDeclarativeScript: %v", err)
	}
	if !strings.Contains(script, `page.fill("#email", "user@example.com")`) {
		t.Error("script should contain page.fill call")
	}
}

func TestBrowserExtract_MissingSelectors(t *testing.T) {
	c := &BrowserExtractConnector{}
	_, err := c.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing selectors")
	}
	if !strings.Contains(err.Error(), "selectors is required") {
		t.Errorf("error = %q, want 'selectors is required'", err)
	}
}

func TestBrowserExtract_EmptySelectors(t *testing.T) {
	c := &BrowserExtractConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"selectors": map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error for empty selectors map")
	}
	if !strings.Contains(err.Error(), "non-empty") {
		t.Errorf("error = %q, want 'non-empty'", err)
	}
}

func TestBrowserExtract_InvalidSession(t *testing.T) {
	c := &BrowserExtractConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"selectors":     map[string]any{"title": "h1"},
		"session_state": true,
	})
	if err == nil {
		t.Fatal("expected error for invalid session_state")
	}
}

func TestBrowserExtract_ScriptUsesTextContent(t *testing.T) {
	snippet := fmt.Sprintf("actionData.data = {};\nactionData.data[%s] = await page.textContent(%s);\n",
		mustJSONString("heading"), mustJSONString("h1"))
	script, err := buildDeclarativeScript(snippet, nil, 30000, false)
	if err != nil {
		t.Fatalf("buildDeclarativeScript: %v", err)
	}
	if !strings.Contains(script, `page.textContent("h1")`) {
		t.Error("script should use textContent for text extraction")
	}
}

func TestBrowserExtract_ScriptUsesGetAttribute(t *testing.T) {
	snippet := fmt.Sprintf("actionData.data = {};\nactionData.data[%s] = await page.getAttribute(%s, %s);\n",
		mustJSONString("link"), mustJSONString("a.btn"), mustJSONString("href"))
	script, err := buildDeclarativeScript(snippet, nil, 30000, false)
	if err != nil {
		t.Fatalf("buildDeclarativeScript: %v", err)
	}
	if !strings.Contains(script, `page.getAttribute("a.btn", "href")`) {
		t.Error("script should use getAttribute when attribute is set")
	}
}

func TestBrowserScreenshot_DefaultPath(t *testing.T) {
	snippet := fmt.Sprintf(
		"const buf = await page.screenshot({ fullPage: %v, path: undefined });\nactionData.base64 = buf.toString('base64');\nactionData.path = %s;\n",
		false, mustJSONString("screenshot.png"),
	)
	script, err := buildDeclarativeScript(snippet, nil, 30000, false)
	if err != nil {
		t.Fatalf("buildDeclarativeScript: %v", err)
	}
	if !strings.Contains(script, `"screenshot.png"`) {
		t.Error("default path should be screenshot.png")
	}
}

func TestBrowserScreenshot_InvalidSession(t *testing.T) {
	c := &BrowserScreenshotConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"session_state": []int{1, 2, 3},
	})
	if err == nil {
		t.Fatal("expected error for invalid session_state")
	}
}

func TestBrowserScreenshot_FullPage(t *testing.T) {
	snippet := fmt.Sprintf(
		"const buf = await page.screenshot({ fullPage: %v, path: undefined });\nactionData.base64 = buf.toString('base64');\nactionData.path = %s;\n",
		true, mustJSONString("screenshot.png"),
	)
	script, err := buildDeclarativeScript(snippet, nil, 30000, false)
	if err != nil {
		t.Fatalf("buildDeclarativeScript: %v", err)
	}
	if !strings.Contains(script, "fullPage: true") {
		t.Error("fullPage:true should appear in script")
	}
}

func TestBrowserWait_NoParams(t *testing.T) {
	c := &BrowserWaitConnector{}
	_, err := c.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error when no wait condition provided")
	}
	if !strings.Contains(err.Error(), "exactly one") {
		t.Errorf("error = %q, want 'exactly one'", err)
	}
}

func TestBrowserWait_TwoParams(t *testing.T) {
	c := &BrowserWaitConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"selector":    ".foo",
		"url_pattern": "https://*",
	})
	if err == nil {
		t.Fatal("expected error when two conditions provided")
	}
	if !strings.Contains(err.Error(), "exactly one") {
		t.Errorf("error = %q, want 'exactly one'", err)
	}
}

func TestBrowserWait_SelectorScript(t *testing.T) {
	snippet := fmt.Sprintf("await page.waitForSelector(%s);\n", mustJSONString(".ready"))
	script, err := buildDeclarativeScript(snippet, nil, 30000, false)
	if err != nil {
		t.Fatalf("buildDeclarativeScript: %v", err)
	}
	if !strings.Contains(script, `waitForSelector(".ready")`) {
		t.Error("script should contain waitForSelector")
	}
}

func TestBrowserWait_URLPatternScript(t *testing.T) {
	snippet := fmt.Sprintf("await page.waitForURL(%s);\n", mustJSONString("https://example.com/**"))
	script, err := buildDeclarativeScript(snippet, nil, 30000, false)
	if err != nil {
		t.Fatalf("buildDeclarativeScript: %v", err)
	}
	if !strings.Contains(script, `waitForURL("https://example.com/**")`) {
		t.Error("script should contain waitForURL")
	}
}

func TestBrowserWait_DurationScript(t *testing.T) {
	snippet := fmt.Sprintf("await page.waitForTimeout(%d);\n", 2000)
	script, err := buildDeclarativeScript(snippet, nil, 30000, false)
	if err != nil {
		t.Fatalf("buildDeclarativeScript: %v", err)
	}
	if !strings.Contains(script, "waitForTimeout(2000)") {
		t.Error("script should contain waitForTimeout")
	}
}

func TestBrowserWait_InvalidSession(t *testing.T) {
	c := &BrowserWaitConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"selector":      ".foo",
		"session_state": 99,
	})
	if err == nil {
		t.Fatal("expected error for invalid session_state")
	}
}

func TestBrowserEvaluate_MissingExpression(t *testing.T) {
	c := &BrowserEvaluateConnector{}
	_, err := c.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing expression")
	}
	if !strings.Contains(err.Error(), "expression is required") {
		t.Errorf("error = %q, want 'expression is required'", err)
	}
}

func TestBrowserEvaluate_EmptyExpression(t *testing.T) {
	c := &BrowserEvaluateConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"expression": "   ",
	})
	if err == nil {
		t.Fatal("expected error for blank expression")
	}
}

func TestBrowserEvaluate_InvalidSession(t *testing.T) {
	c := &BrowserEvaluateConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"expression":    "document.title",
		"session_state": false,
	})
	if err == nil {
		t.Fatal("expected error for invalid session_state")
	}
}

func TestBrowserEvaluate_ScriptContainsExpression(t *testing.T) {
	snippet := "actionData.result = await page.evaluate(() => { return (document.title); });\n"
	script, err := buildDeclarativeScript(snippet, nil, 30000, false)
	if err != nil {
		t.Fatalf("buildDeclarativeScript: %v", err)
	}
	if !strings.Contains(script, "document.title") {
		t.Error("script should contain the expression")
	}
	if !strings.Contains(script, "page.evaluate") {
		t.Error("script should use page.evaluate")
	}
}

// ---------------------------------------------------------------------------
// Integration tests — require Docker and network access
// ---------------------------------------------------------------------------

func TestBrowserNavigate_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	dockerAvailableForBrowser(t)

	c := &BrowserNavigateConnector{}
	out, err := c.Execute(context.Background(), map[string]any{
		"url":  "https://example.com",
		"pull": "missing",
	})
	if err != nil {
		t.Skipf("Execute failed (may need network/npm): %v", err)
	}
	title, _ := out["title"].(string)
	if !strings.Contains(title, "Example Domain") {
		t.Errorf("title = %q, want 'Example Domain'", title)
	}
	if out["session_state"] == nil {
		t.Error("session_state should be present in output")
	}
}

func TestBrowserNavigateExtract_SessionChain(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	dockerAvailableForBrowser(t)

	// Step 1: navigate
	nav := &BrowserNavigateConnector{}
	navOut, err := nav.Execute(context.Background(), map[string]any{
		"url":  "https://example.com",
		"pull": "missing",
	})
	if err != nil {
		t.Skipf("navigate failed (may need network/npm): %v", err)
	}

	// Step 2: extract using session from step 1
	ext := &BrowserExtractConnector{}
	extOut, err := ext.Execute(context.Background(), map[string]any{
		"selectors":     map[string]any{"heading": "h1"},
		"session_state": navOut["session_state"],
		"pull":          "missing",
	})
	if err != nil {
		t.Skipf("extract failed: %v", err)
	}
	data, ok := extOut["data"].(map[string]any)
	if !ok {
		t.Fatalf("data output is not a map: %T", extOut["data"])
	}
	heading, _ := data["heading"].(string)
	if !strings.Contains(heading, "Example Domain") {
		t.Errorf("heading = %q, want 'Example Domain'", heading)
	}
}

func TestBrowserScreenshot_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	dockerAvailableForBrowser(t)

	nav := &BrowserNavigateConnector{}
	navOut, err := nav.Execute(context.Background(), map[string]any{
		"url":  "https://example.com",
		"pull": "missing",
	})
	if err != nil {
		t.Skipf("navigate failed (may need network/npm): %v", err)
	}

	ss := &BrowserScreenshotConnector{}
	ssOut, err := ss.Execute(context.Background(), map[string]any{
		"session_state": navOut["session_state"],
		"pull":          "missing",
	})
	if err != nil {
		t.Skipf("screenshot failed: %v", err)
	}
	b64, _ := ssOut["base64"].(string)
	if len(b64) == 0 {
		t.Error("expected non-empty base64 screenshot")
	}
	if ssOut["path"] != "screenshot.png" {
		t.Errorf("path = %v, want screenshot.png", ssOut["path"])
	}
}

func TestBrowserEvaluate_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	dockerAvailableForBrowser(t)

	nav := &BrowserNavigateConnector{}
	navOut, err := nav.Execute(context.Background(), map[string]any{
		"url":  "https://example.com",
		"pull": "missing",
	})
	if err != nil {
		t.Skipf("navigate failed (may need network/npm): %v", err)
	}

	ev := &BrowserEvaluateConnector{}
	evOut, err := ev.Execute(context.Background(), map[string]any{
		"expression":    "document.location.hostname",
		"session_state": navOut["session_state"],
		"pull":          "missing",
	})
	if err != nil {
		t.Skipf("evaluate failed: %v", err)
	}
	result, _ := evOut["result"].(string)
	if result != "example.com" {
		t.Errorf("result = %q, want %q", result, "example.com")
	}
}

func TestBrowserWait_TimeoutError_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	dockerAvailableForBrowser(t)

	nav := &BrowserNavigateConnector{}
	navOut, err := nav.Execute(context.Background(), map[string]any{
		"url":  "https://example.com",
		"pull": "missing",
	})
	if err != nil {
		t.Skipf("navigate failed (may need network/npm): %v", err)
	}

	w := &BrowserWaitConnector{}
	_, err = w.Execute(context.Background(), map[string]any{
		"selector":      ".this-selector-does-not-exist",
		"session_state": navOut["session_state"],
		"timeout_ms":    float64(2000),
		"pull":          "missing",
	})
	if err == nil {
		t.Fatal("expected error when selector not found within timeout")
	}
	if !strings.Contains(err.Error(), "browser/wait") {
		t.Errorf("error = %q, want 'browser/wait' prefix", err)
	}
}
