package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	playwrightNodeImage   = "mcr.microsoft.com/playwright:v1.52.0-noble"
	playwrightPythonImage = "mcr.microsoft.com/playwright/python:v1.52.0-noble"
)

// BrowserRunConnector wraps user Playwright scripts with boilerplate and
// delegates container execution to DockerRunConnector.
type BrowserRunConnector struct{}

// buildJSWrapper wraps the user's JavaScript or TypeScript Playwright script
// with boilerplate that launches Chromium and tears it down.
func buildJSWrapper(userScript string) string {
	return "const { chromium } = require('playwright');\n" +
		"(async () => {\n" +
		"    const browser = await chromium.launch();\n" +
		"    try {\n" +
		"        " + userScript + "\n" +
		"    } finally {\n" +
		"        await browser.close();\n" +
		"    }\n" +
		"})();\n"
}

// buildPythonWrapper wraps the user's Python Playwright script with boilerplate
// that launches Chromium and tears it down.
func buildPythonWrapper(userScript string) string {
	// Indent each line of the user script by 8 spaces to sit inside the try block.
	indented := indentLines(userScript, "        ")
	return "from playwright.sync_api import sync_playwright\n" +
		"with sync_playwright() as p:\n" +
		"    browser = p.chromium.launch()\n" +
		"    try:\n" +
		"        page = browser.new_page()\n" +
		indented + "\n" +
		"    finally:\n" +
		"        browser.close()\n"
}

// indentLines prefixes each non-empty line of s with the given prefix.
// Empty lines are left empty to avoid trailing whitespace.
func indentLines(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
}

// Execute implements Connector. Params:
//   - language      (string)           "javascript" | "typescript" | "python" (default "javascript")
//   - script        (string, required) the user's Playwright script
//   - output_format (string)           "json" | "text" (default "text")
//   - env           (map[string]string) environment variables
//   - pull          (string)           image pull policy passed to docker/run
//   - memory        (string)           memory limit (default "1g")
//   - _credential   (any)              passed to docker/run for Docker daemon access
func (c *BrowserRunConnector) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	// --- language ---
	language, _ := params["language"].(string)
	if language == "" {
		language = "javascript"
	}
	switch language {
	case "javascript", "typescript", "python":
		// valid
	default:
		return nil, fmt.Errorf("browser/run: unsupported language %q (must be javascript, typescript, or python)", language)
	}

	// --- script ---
	script, _ := params["script"].(string)
	if strings.TrimSpace(script) == "" {
		return nil, fmt.Errorf("browser/run: script is required")
	}

	// --- output_format ---
	outputFormat, _ := params["output_format"].(string)
	if outputFormat != "" && outputFormat != "json" && outputFormat != "text" {
		return nil, fmt.Errorf("browser/run: output_format must be 'json' or 'text', got %q", outputFormat)
	}

	// --- memory ---
	memory, _ := params["memory"].(string)
	if memory == "" {
		memory = "1g"
	}

	// --- select image and build container command ---
	var (
		containerImage string
		wrapperScript  string
		containerCmd   []string
	)

	switch language {
	case "javascript":
		containerImage = playwrightNodeImage
		wrapperScript = buildJSWrapper(script)
		// The Playwright Docker image ships browsers but not the npm package.
		// Install it silently before running so the script can require('playwright').
		// `node` with no file argument reads the script from stdin.
		containerCmd = []string{"sh", "-c", "cd /tmp && npm init -y --silent 2>/dev/null && npm install --silent playwright 2>/dev/null && node"}
	case "typescript":
		containerImage = playwrightNodeImage
		wrapperScript = buildJSWrapper(script)
		// Same npm-install preamble; use --experimental-strip-types for TS syntax.
		containerCmd = []string{"sh", "-c", "cd /tmp && npm init -y --silent 2>/dev/null && npm install --silent playwright 2>/dev/null && node --experimental-strip-types"}
	case "python":
		containerImage = playwrightPythonImage
		wrapperScript = buildPythonWrapper(script)
		// Install the playwright Python package if not already present, then run
		// the script via stdin (`python3 -`).
		containerCmd = []string{"sh", "-c", "pip install --quiet playwright 2>/dev/null && python3 -"}
	}

	// --- build docker/run params ---
	dockerParams := map[string]any{
		"image":   containerImage,
		"cmd":     toAnySlice(containerCmd),
		"stdin":   wrapperScript,
		"network": "bridge",
		"remove":  true,
		"memory":  memory,
	}

	// Pass through optional params.
	if pull, ok := params["pull"].(string); ok && pull != "" {
		dockerParams["pull"] = pull
	}
	if cred, ok := params["_credential"]; ok {
		dockerParams["_credential"] = cred
	}
	if envRaw, ok := params["env"]; ok {
		dockerParams["env"] = envRaw
	}

	// --- delegate to DockerRunConnector ---
	docker := &DockerRunConnector{}
	result, err := docker.Execute(ctx, dockerParams)
	if err != nil {
		return nil, fmt.Errorf("browser/run: %w", err)
	}

	// --- build output ---
	output := map[string]any{
		"exit_code": result["exit_code"],
		"stdout":    result["stdout"],
		"stderr":    result["stderr"],
	}

	// Optionally parse stdout as JSON.
	if outputFormat == "json" {
		stdout, _ := result["stdout"].(string)
		stdout = strings.TrimSpace(stdout)
		if stdout != "" {
			var parsed any
			if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
				return nil, fmt.Errorf("browser/run: output_format is 'json' but stdout is not valid JSON: %w", err)
			}
			output["json"] = parsed
		}
	}

	return output, nil
}

// toAnySlice converts []string to []any for use in dockerParams["cmd"].
func toAnySlice(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}
