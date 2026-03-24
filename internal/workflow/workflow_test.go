package workflow

import "testing"

func TestValidationError_Error(t *testing.T) {
	e := ValidationError{
		Line:    10,
		Column:  3,
		Field:   "name",
		Message: "name is required",
	}

	got := e.Error()
	want := "10:3: error: name is required (name)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestValidationError_ZeroValues(t *testing.T) {
	e := ValidationError{
		Field:   "steps",
		Message: "steps required",
	}

	got := e.Error()
	want := "0:0: error: steps required (steps)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWorkflowStructTags(t *testing.T) {
	// Verify that the structs can be instantiated with expected fields.
	w := Workflow{
		Name:        "test",
		Description: "a test workflow",
		Inputs: map[string]Input{
			"url": {Type: "string", Description: "target URL"},
		},
		Steps: []Step{
			{
				Name:   "fetch",
				Action: "http.request",
				Params: map[string]any{"url": "https://example.com"},
				If:     "inputs.url != ''",
				Retry:  &RetryPolicy{MaxAttempts: 3, Backoff: "exponential"},
				Timeout: "30s",
			},
		},
	}

	if w.Name != "test" {
		t.Errorf("expected name 'test', got %q", w.Name)
	}
	if len(w.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(w.Steps))
	}
	if w.Steps[0].Retry.MaxAttempts != 3 {
		t.Errorf("expected max_attempts 3, got %d", w.Steps[0].Retry.MaxAttempts)
	}
}

func TestParse_TokenBudget(t *testing.T) {
	input := []byte(`
name: test-workflow
token_budget: 500000
steps:
  - name: step1
    action: http/request
    params:
      url: "https://example.com"
`)
	result, err := ParseBytes(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Workflow.TokenBudget != int64(500000) {
		t.Errorf("token_budget: got %d, want 500000", result.Workflow.TokenBudget)
	}
}

func TestParse_TokenBudget_Zero(t *testing.T) {
	input := []byte(`
name: test-workflow
steps:
  - name: step1
    action: http/request
    params:
      url: "https://example.com"
`)
	result, err := ParseBytes(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Workflow.TokenBudget != int64(0) {
		t.Errorf("token_budget: got %d, want 0", result.Workflow.TokenBudget)
	}
}
