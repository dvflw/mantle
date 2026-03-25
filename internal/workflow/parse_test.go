package workflow

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	return path
}

func TestParse_ValidWorkflow(t *testing.T) {
	path := writeTestFile(t, `
name: my-workflow
description: A test workflow
inputs:
  url:
    type: string
    description: Target URL
steps:
  - name: fetch-data
    action: http.request
    params:
      method: GET
      url: "https://example.com"
`)

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Workflow == nil {
		t.Fatal("workflow is nil")
	}
	if result.Root == nil {
		t.Fatal("root node is nil")
	}

	w := result.Workflow
	if w.Name != "my-workflow" {
		t.Errorf("name: got %q, want %q", w.Name, "my-workflow")
	}
	if w.Description != "A test workflow" {
		t.Errorf("description: got %q, want %q", w.Description, "A test workflow")
	}
	if len(w.Inputs) != 1 {
		t.Fatalf("inputs: got %d, want 1", len(w.Inputs))
	}
	inp, ok := w.Inputs["url"]
	if !ok {
		t.Fatal("missing input 'url'")
	}
	if inp.Type != "string" {
		t.Errorf("input type: got %q, want %q", inp.Type, "string")
	}
	if inp.Description != "Target URL" {
		t.Errorf("input description: got %q, want %q", inp.Description, "Target URL")
	}
	if len(w.Steps) != 1 {
		t.Fatalf("steps: got %d, want 1", len(w.Steps))
	}
	step := w.Steps[0]
	if step.Name != "fetch-data" {
		t.Errorf("step name: got %q, want %q", step.Name, "fetch-data")
	}
	if step.Action != "http.request" {
		t.Errorf("step action: got %q, want %q", step.Action, "http.request")
	}
	if step.Params["method"] != "GET" {
		t.Errorf("step param method: got %v, want %q", step.Params["method"], "GET")
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	path := writeTestFile(t, `
name: test
steps:
  - name: [invalid
`)

	_, err := Parse(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestParse_FileNotFound(t *testing.T) {
	_, err := Parse("/nonexistent/path/workflow.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestParse_WithRetryAndTimeout(t *testing.T) {
	path := writeTestFile(t, `
name: retry-workflow
steps:
  - name: flaky-step
    action: http.request
    timeout: "30s"
    retry:
      max_attempts: 5
      backoff: exponential
`)

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	step := result.Workflow.Steps[0]
	if step.Timeout != "30s" {
		t.Errorf("timeout: got %q, want %q", step.Timeout, "30s")
	}
	if step.Retry == nil {
		t.Fatal("retry is nil")
	}
	if step.Retry.MaxAttempts != 5 {
		t.Errorf("max_attempts: got %d, want 5", step.Retry.MaxAttempts)
	}
	if step.Retry.Backoff != "exponential" {
		t.Errorf("backoff: got %q, want %q", step.Retry.Backoff, "exponential")
	}
}

func TestParse_WithConditional(t *testing.T) {
	path := writeTestFile(t, `
name: conditional-workflow
steps:
  - name: maybe-run
    action: http.request
    if: "inputs.enabled == true"
`)

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	step := result.Workflow.Steps[0]
	if step.If != "inputs.enabled == true" {
		t.Errorf("if: got %q, want %q", step.If, "inputs.enabled == true")
	}
}

func TestParseTools(t *testing.T) {
	params := map[string]any{
		"model":  "gpt-4",
		"prompt": "Help the user",
		"tools": []any{
			map[string]any{
				"name":        "get_weather",
				"description": "Get current weather for a location",
				"input_schema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"location": map[string]any{
							"type":        "string",
							"description": "City name",
						},
					},
					"required": []any{"location"},
				},
				"action": "http.request",
				"params": map[string]any{
					"method": "GET",
					"url":    "https://api.weather.com/${input.location}",
				},
			},
		},
	}

	tools, err := ParseTools(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("tools: got %d, want 1", len(tools))
	}

	tool := tools[0]
	if tool.Name != "get_weather" {
		t.Errorf("name: got %q, want %q", tool.Name, "get_weather")
	}
	if tool.Description != "Get current weather for a location" {
		t.Errorf("description: got %q, want %q", tool.Description, "Get current weather for a location")
	}
	if tool.Action != "http.request" {
		t.Errorf("action: got %q, want %q", tool.Action, "http.request")
	}
	if tool.InputSchema == nil {
		t.Fatal("input_schema is nil")
	}
	if tool.InputSchema["type"] != "object" {
		t.Errorf("input_schema type: got %v, want %q", tool.InputSchema["type"], "object")
	}
	if tool.Params == nil {
		t.Fatal("params is nil")
	}
	if tool.Params["method"] != "GET" {
		t.Errorf("params method: got %v, want %q", tool.Params["method"], "GET")
	}
}

func TestParseTools_NoTools(t *testing.T) {
	params := map[string]any{
		"model":  "gpt-4",
		"prompt": "Hello",
	}

	tools, err := ParseTools(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tools != nil {
		t.Fatalf("tools: got %v, want nil", tools)
	}
}

func TestParseTools_InvalidType(t *testing.T) {
	params := map[string]any{
		"tools": "not-an-array",
	}

	_, err := ParseTools(params)
	if err == nil {
		t.Fatal("expected error for non-array tools")
	}
}

func TestParse_ToolUseStep(t *testing.T) {
	path := writeTestFile(t, `
name: tool-use-workflow
description: Workflow with AI tool use
steps:
  - name: ai-with-tools
    action: ai.completion
    params:
      model: gpt-4
      prompt: "Help the user check the weather"
      tools:
        - name: get_weather
          description: Get current weather
          input_schema:
            type: object
            properties:
              location:
                type: string
          action: http.request
          params:
            method: GET
            url: "https://api.weather.com"
        - name: get_forecast
          description: Get weather forecast
          input_schema:
            type: object
            properties:
              location:
                type: string
              days:
                type: integer
          action: http.request
          params:
            method: GET
            url: "https://api.weather.com/forecast"
`)

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	step := result.Workflow.Steps[0]
	if step.Name != "ai-with-tools" {
		t.Errorf("step name: got %q, want %q", step.Name, "ai-with-tools")
	}
	if step.Action != "ai.completion" {
		t.Errorf("step action: got %q, want %q", step.Action, "ai.completion")
	}

	tools, err := ParseTools(step.Params)
	if err != nil {
		t.Fatalf("ParseTools error: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("tools: got %d, want 2", len(tools))
	}

	if tools[0].Name != "get_weather" {
		t.Errorf("tool[0] name: got %q, want %q", tools[0].Name, "get_weather")
	}
	if tools[0].Action != "http.request" {
		t.Errorf("tool[0] action: got %q, want %q", tools[0].Action, "http.request")
	}
	if tools[1].Name != "get_forecast" {
		t.Errorf("tool[1] name: got %q, want %q", tools[1].Name, "get_forecast")
	}
	if tools[1].InputSchema == nil {
		t.Fatal("tool[1] input_schema is nil")
	}
}

func TestParse_ArtifactsAndRegistryCredential(t *testing.T) {
	path := writeTestFile(t, `
name: test-artifacts
description: test artifacts parsing
steps:
  - name: build
    action: docker/run
    credential: my-docker
    registry_credential: my-registry
    params:
      image: alpine
      cmd: ["tar", "-czf", "/mantle/artifacts/out.tar.gz", "/data"]
    artifacts:
      - path: /mantle/artifacts/out.tar.gz
        name: build-output
`)
	result, err := Parse(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	step := result.Workflow.Steps[0]
	if step.RegistryCredential != "my-registry" {
		t.Errorf("registry_credential = %q, want %q", step.RegistryCredential, "my-registry")
	}
	if len(step.Artifacts) != 1 {
		t.Fatalf("len(artifacts) = %d, want 1", len(step.Artifacts))
	}
	if step.Artifacts[0].Name != "build-output" {
		t.Errorf("artifact name = %q, want %q", step.Artifacts[0].Name, "build-output")
	}
	if step.Artifacts[0].Path != "/mantle/artifacts/out.tar.gz" {
		t.Errorf("artifact path = %q, want %q", step.Artifacts[0].Path, "/mantle/artifacts/out.tar.gz")
	}
}
