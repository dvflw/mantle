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
