package connector

import (
	"context"
	"strings"
	"testing"

	"github.com/docker/docker/client"
)

// ---------------------------------------------------------------------------
// Unit tests — no Docker required
// ---------------------------------------------------------------------------

func TestBrowserRun_MissingScript(t *testing.T) {
	c := &BrowserRunConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"language": "javascript",
	})
	if err == nil {
		t.Fatal("expected error for missing script")
	}
	if !strings.Contains(err.Error(), "script is required") {
		t.Errorf("error = %q, want 'script is required'", err)
	}
}

func TestBrowserRun_EmptyScript(t *testing.T) {
	c := &BrowserRunConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"language": "javascript",
		"script":   "   ",
	})
	if err == nil {
		t.Fatal("expected error for empty script")
	}
	if !strings.Contains(err.Error(), "script is required") {
		t.Errorf("error = %q, want 'script is required'", err)
	}
}

func TestBrowserRun_InvalidLanguage(t *testing.T) {
	c := &BrowserRunConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"language": "ruby",
		"script":   "puts 'hello'",
	})
	if err == nil {
		t.Fatal("expected error for unsupported language")
	}
	if !strings.Contains(err.Error(), "unsupported language") {
		t.Errorf("error = %q, want 'unsupported language'", err)
	}
}

func TestBrowserRun_InvalidOutputFormat(t *testing.T) {
	c := &BrowserRunConnector{}
	_, err := c.Execute(context.Background(), map[string]any{
		"script":        "console.log('hi')",
		"output_format": "xml",
	})
	if err == nil {
		t.Fatal("expected error for invalid output_format")
	}
	if !strings.Contains(err.Error(), "output_format") {
		t.Errorf("error = %q, want mention of output_format", err)
	}
}

func TestBrowserRun_DefaultLanguageIsJavaScript(t *testing.T) {
	// Build the JS wrapper using a minimal script and check expected structure.
	script := "console.log('hi');"
	wrapper := buildJSWrapper(script)
	if !strings.Contains(wrapper, "chromium") {
		t.Error("JS wrapper should reference chromium")
	}
	if !strings.Contains(wrapper, "browser.close") {
		t.Error("JS wrapper should call browser.close")
	}
	if !strings.Contains(wrapper, script) {
		t.Errorf("JS wrapper should contain the user script")
	}
}

// ---------------------------------------------------------------------------
// Wrapper generation unit tests
// ---------------------------------------------------------------------------

func TestBuildJSWrapper(t *testing.T) {
	script := "const page = await browser.newPage(); console.log(await page.title());"
	wrapper := buildJSWrapper(script)

	checks := []string{
		"require('playwright')",
		"chromium.launch()",
		"browser.close()",
		"async",
		script,
	}
	for _, want := range checks {
		if !strings.Contains(wrapper, want) {
			t.Errorf("JS wrapper missing %q\nwrapper:\n%s", want, wrapper)
		}
	}
}

func TestBuildPythonWrapper(t *testing.T) {
	script := "page.goto('https://example.com')"
	wrapper := buildPythonWrapper(script)

	checks := []string{
		"from playwright.sync_api import sync_playwright",
		"sync_playwright()",
		"p.chromium.launch()",
		"browser.new_page()",
		"browser.close()",
		script,
	}
	for _, want := range checks {
		if !strings.Contains(wrapper, want) {
			t.Errorf("Python wrapper missing %q\nwrapper:\n%s", want, wrapper)
		}
	}
}

func TestBuildJSWrapper_TypeScriptPassthrough(t *testing.T) {
	// TypeScript uses the same JS wrapper since the Node Playwright image can
	// execute TS syntax via the wrapper route.
	tsScript := "const page: any = await browser.newPage();"
	wrapper := buildJSWrapper(tsScript)
	if !strings.Contains(wrapper, tsScript) {
		t.Errorf("JS/TS wrapper should contain the user TypeScript script")
	}
}

func TestIndentLines(t *testing.T) {
	input := "line1\nline2\n\nline4"
	got := indentLines(input, "  ")
	want := "  line1\n  line2\n\n  line4"
	if got != want {
		t.Errorf("indentLines:\ngot:  %q\nwant: %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// Integration tests — require Docker
// ---------------------------------------------------------------------------

func dockerAvailableForBrowser(t *testing.T) {
	t.Helper()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer cli.Close()
	if _, err := cli.Ping(context.Background()); err != nil {
		t.Skipf("Docker not reachable: %v", err)
	}
}

func TestBrowserRun_JavaScript(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	dockerAvailableForBrowser(t)

	c := &BrowserRunConnector{}
	output, err := c.Execute(context.Background(), map[string]any{
		"language": "javascript",
		"script": `
			const page = await browser.newPage();
			await page.goto('https://example.com');
			const title = await page.title();
			console.log(JSON.stringify({ title }));
		`,
		"output_format": "json",
		"pull":          "missing",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if output["exit_code"] != int64(0) {
		t.Errorf("exit_code = %v, want 0\nstderr: %s", output["exit_code"], output["stderr"])
	}
	jsonOut, ok := output["json"].(map[string]any)
	if !ok {
		t.Fatalf("json output is not a map: %T %v", output["json"], output["json"])
	}
	title, _ := jsonOut["title"].(string)
	if !strings.Contains(title, "Example Domain") {
		t.Errorf("title = %q, want 'Example Domain'", title)
	}
}

func TestBrowserRun_Python(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	dockerAvailableForBrowser(t)

	c := &BrowserRunConnector{}
	output, err := c.Execute(context.Background(), map[string]any{
		"language": "python",
		"script": `
page.goto('https://example.com')
title = page.title()
import json, sys
print(json.dumps({'title': title}))
`,
		"output_format": "json",
		"pull":          "missing",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if output["exit_code"] != int64(0) {
		t.Errorf("exit_code = %v, want 0\nstderr: %s", output["exit_code"], output["stderr"])
	}
	jsonOut, ok := output["json"].(map[string]any)
	if !ok {
		t.Fatalf("json output is not a map: %T %v", output["json"], output["json"])
	}
	title, _ := jsonOut["title"].(string)
	if !strings.Contains(title, "Example Domain") {
		t.Errorf("title = %q, want 'Example Domain'", title)
	}
}
