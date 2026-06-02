package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// BrowserSession holds serialized browser state passed between declarative steps.
type BrowserSession struct {
	Cookies      []map[string]any             `json:"cookies"`
	LocalStorage map[string]map[string]string `json:"local_storage"`
	URL          string                       `json:"url"`
}

// extractSession parses the optional session_state param into a *BrowserSession.
// Returns nil (no error) when absent or nil — callers treat nil as "start fresh".
func extractSession(params map[string]any) (*BrowserSession, error) {
	raw, ok := params["session_state"]
	if !ok || raw == nil {
		return nil, nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("browser: marshaling session_state: %w", err)
	}
	var s BrowserSession
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("browser: invalid session_state: %w", err)
	}
	return &s, nil
}

// buildDeclarativeScript generates a self-contained Playwright JS script.
//
// actionSnippet is injected into the page context after optional session
// restore. skipURLRestore=true restores cookies but skips navigating to the
// previous URL — used by browser/navigate, which sets its own destination.
func buildDeclarativeScript(actionSnippet string, session *BrowserSession, timeoutMs int, skipURLRestore bool) (string, error) {
	var b strings.Builder

	b.WriteString("const { chromium } = require('playwright');\n")
	b.WriteString("(async () => {\n")
	b.WriteString("  let actionData = {};\n")
	b.WriteString("  const browser = await chromium.launch({ headless: true });\n")
	b.WriteString("  try {\n")
	b.WriteString("    const context = await browser.newContext();\n")

	if session != nil && len(session.Cookies) > 0 {
		cookiesJSON, err := json.Marshal(session.Cookies)
		if err != nil {
			return "", fmt.Errorf("browser: marshaling cookies: %w", err)
		}
		fmt.Fprintf(&b, "    await context.addCookies(%s);\n", cookiesJSON)
	}

	b.WriteString("    const page = await context.newPage();\n")
	fmt.Fprintf(&b, "    page.setDefaultTimeout(%d);\n", timeoutMs)

	if session != nil && session.URL != "" && !skipURLRestore {
		urlJSON, err := json.Marshal(session.URL)
		if err != nil {
			return "", fmt.Errorf("browser: marshaling URL: %w", err)
		}
		fmt.Fprintf(&b, "    await page.goto(%s);\n", urlJSON)
		// Note: localStorage is keyed by origin. If the page at session.URL redirects
		// to a different origin, window.location.origin after navigation won't match
		// the stored key and localStorage will silently not be restored. This is an
		// inherent limitation of post-navigation injection; use addInitScript for
		// redirect-safe restore if this becomes an issue.
		if len(session.LocalStorage) > 0 {
			lsJSON, err := json.Marshal(session.LocalStorage)
			if err != nil {
				return "", fmt.Errorf("browser: marshaling localStorage: %w", err)
			}
			fmt.Fprintf(&b, "    await page.evaluate((ls) => {\n")
			b.WriteString("      const origin = window.location.origin;\n")
			b.WriteString("      for (const [k, v] of Object.entries(ls[origin] || {})) localStorage.setItem(k, v);\n")
			fmt.Fprintf(&b, "    }, %s);\n", lsJSON)
		}
	}

	// Inject action snippet with consistent indentation.
	for _, line := range strings.Split(strings.TrimRight(actionSnippet, "\n"), "\n") {
		fmt.Fprintf(&b, "    %s\n", line)
	}

	// Capture session state after the action.
	b.WriteString("    const _outCookies = await context.cookies();\n")
	b.WriteString("    const _outLS = {};\n")
	b.WriteString("    const _url = page.url();\n")
	b.WriteString("    try {\n")
	b.WriteString("      const _origin = new URL(_url).origin;\n")
	b.WriteString("      _outLS[_origin] = await page.evaluate(() => {\n")
	b.WriteString("        const d = {};\n")
	b.WriteString("        for (let i = 0; i < localStorage.length; i++) {\n")
	b.WriteString("          const k = localStorage.key(i); d[k] = localStorage.getItem(k);\n")
	b.WriteString("        }\n")
	b.WriteString("        return d;\n")
	b.WriteString("      });\n")
	b.WriteString("    } catch (_) {}\n")
	b.WriteString("    console.log(JSON.stringify({\n")
	b.WriteString("      session_state: { cookies: _outCookies, local_storage: _outLS, url: _url },\n")
	b.WriteString("      data: actionData\n")
	b.WriteString("    }));\n")
	b.WriteString("  } finally {\n")
	b.WriteString("    await browser.close();\n")
	b.WriteString("  }\n")
	b.WriteString("})().catch(err => { process.stderr.write(err.message + '\\n'); process.exit(1); });\n")

	return b.String(), nil
}

// extractTimeoutMs returns the timeout_ms param as an int, defaulting to 30000.
func extractTimeoutMs(params map[string]any) int {
	if v, ok := params["timeout_ms"]; ok {
		switch t := v.(type) {
		case int:
			return t
		case int64:
			return int(t)
		case float64:
			return int(t)
		}
	}
	return 30000
}

// executeBrowserScript runs a generated Playwright script via DockerRunConnector
// and returns the parsed JSON envelope { session_state, data }.
//
// Passes through pull, memory, and _credential from params.
func executeBrowserScript(ctx context.Context, script string, params map[string]any) (map[string]any, error) {
	memory, _ := params["memory"].(string)
	if memory == "" {
		memory = "1g"
	}
	// playwrightNodeImage and playwrightVersion are defined in browser.go (same package).
	dockerParams := map[string]any{
		"image": playwrightNodeImage,
		"cmd": toAnySlice([]string{
			"sh", "-c",
			"npm install --no-save --silent playwright@" + playwrightVersion + " && node",
		}),
		"stdin":   script,
		"network": "bridge",
		"remove":  true,
		"memory":  memory,
	}
	if pull, ok := params["pull"].(string); ok && pull != "" {
		dockerParams["pull"] = pull
	}
	if cred, ok := params["_credential"]; ok {
		dockerParams["_credential"] = cred
	}

	docker := &DockerRunConnector{}
	result, err := docker.Execute(ctx, dockerParams)
	if err != nil {
		return nil, err
	}
	if code, _ := result["exit_code"].(int64); code != 0 {
		stderr, _ := result["stderr"].(string)
		return nil, fmt.Errorf("script failed (exit %d): %s", code, strings.TrimSpace(stderr))
	}
	stdout, _ := result["stdout"].(string)
	var envelope map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &envelope); err != nil {
		return nil, fmt.Errorf("invalid output JSON: %w", err)
	}
	return envelope, nil
}

// mustJSONString returns the JSON encoding of s as a string literal (e.g. "\"hello\"").
// It panics only if json.Marshal fails on a plain string, which cannot happen.
func mustJSONString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// BrowserNavigateConnector implements browser/navigate.
type BrowserNavigateConnector struct{}

func (c *BrowserNavigateConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	url, _ := params["url"].(string)
	if strings.TrimSpace(url) == "" {
		return nil, fmt.Errorf("browser/navigate: url is required")
	}
	waitUntil, _ := params["wait_until"].(string)
	if waitUntil == "" {
		waitUntil = "load"
	}
	switch waitUntil {
	case "load", "networkidle", "domcontentloaded":
	default:
		return nil, fmt.Errorf("browser/navigate: wait_until must be load, networkidle, or domcontentloaded, got %q", waitUntil)
	}
	session, err := extractSession(params)
	if err != nil {
		return nil, fmt.Errorf("browser/navigate: %w", err)
	}
	snippet := fmt.Sprintf(
		"await page.goto(%s, { waitUntil: %s });\nactionData.title = await page.title();",
		mustJSONString(url), mustJSONString(waitUntil),
	)
	script, err := buildDeclarativeScript(snippet, session, extractTimeoutMs(params), true)
	if err != nil {
		return nil, fmt.Errorf("browser/navigate: %w", err)
	}
	envelope, err := executeBrowserScript(ctx, script, params)
	if err != nil {
		return nil, fmt.Errorf("browser/navigate: %w", err)
	}
	out := map[string]any{"session_state": envelope["session_state"]}
	if data, ok := envelope["data"].(map[string]any); ok {
		if v, ok := data["title"]; ok {
			out["title"] = v
		}
	}
	return out, nil
}

// BrowserClickConnector implements browser/click.
type BrowserClickConnector struct{}

func (c *BrowserClickConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	selector, _ := params["selector"].(string)
	if strings.TrimSpace(selector) == "" {
		return nil, fmt.Errorf("browser/click: selector is required")
	}
	waitFor, _ := params["wait_for"].(string)
	session, err := extractSession(params)
	if err != nil {
		return nil, fmt.Errorf("browser/click: %w", err)
	}
	var snippet strings.Builder
	fmt.Fprintf(&snippet, "await page.click(%s);\n", mustJSONString(selector))
	if waitFor != "" {
		fmt.Fprintf(&snippet, "await page.waitForSelector(%s);\n", mustJSONString(waitFor))
	}
	script, err := buildDeclarativeScript(snippet.String(), session, extractTimeoutMs(params), false)
	if err != nil {
		return nil, fmt.Errorf("browser/click: %w", err)
	}
	envelope, err := executeBrowserScript(ctx, script, params)
	if err != nil {
		return nil, fmt.Errorf("browser/click: %w", err)
	}
	return map[string]any{"session_state": envelope["session_state"]}, nil
}
