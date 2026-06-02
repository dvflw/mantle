package connector

import (
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
func buildDeclarativeScript(actionSnippet string, session *BrowserSession, timeoutMs int, skipURLRestore bool) string {
	var b strings.Builder

	b.WriteString("const { chromium } = require('playwright');\n")
	b.WriteString("(async () => {\n")
	b.WriteString("  let actionData = {};\n")
	b.WriteString("  const browser = await chromium.launch();\n")
	b.WriteString("  try {\n")
	b.WriteString("    const context = await browser.newContext();\n")

	if session != nil && len(session.Cookies) > 0 {
		cookiesJSON, _ := json.Marshal(session.Cookies)
		fmt.Fprintf(&b, "    await context.addCookies(%s);\n", cookiesJSON)
	}

	b.WriteString("    const page = await context.newPage();\n")
	fmt.Fprintf(&b, "    page.setDefaultTimeout(%d);\n", timeoutMs)

	if session != nil && session.URL != "" && !skipURLRestore {
		fmt.Fprintf(&b, "    await page.goto(%q);\n", session.URL)
		if len(session.LocalStorage) > 0 {
			lsJSON, _ := json.Marshal(session.LocalStorage)
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

	return b.String()
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
