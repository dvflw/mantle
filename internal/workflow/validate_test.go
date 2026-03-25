package workflow

import (
	"sort"
	"strings"
	"testing"
)

func mustParse(t *testing.T, content string) *ParseResult {
	t.Helper()
	path := writeTestFile(t, content)
	result, err := Parse(path)
	if err != nil {
		t.Fatalf("mustParse failed: %v", err)
	}
	return result
}

func assertHasError(t *testing.T, errs []ValidationError, field string) {
	t.Helper()
	for _, e := range errs {
		if e.Field == field {
			return
		}
	}
	fields := make([]string, len(errs))
	for i, e := range errs {
		fields[i] = e.Field + ": " + e.Message
	}
	t.Errorf("expected error for field %q, got errors: %v", field, fields)
}

func assertErrorContains(t *testing.T, errs []ValidationError, substr string) {
	t.Helper()
	for _, e := range errs {
		if strings.Contains(e.Message, substr) || strings.Contains(e.Field, substr) {
			return
		}
	}
	msgs := make([]string, len(errs))
	for i, e := range errs {
		msgs[i] = e.Field + ": " + e.Message
	}
	t.Errorf("expected error containing %q, got errors: %v", substr, msgs)
}

func assertNoErrors(t *testing.T, errs []ValidationError) {
	t.Helper()
	if len(errs) != 0 {
		for _, e := range errs {
			t.Errorf("unexpected error: %s", e.Error())
		}
		t.Fatalf("expected no errors, got %d", len(errs))
	}
}

func TestValidate_ValidWorkflow(t *testing.T) {
	result := mustParse(t, `
name: my-workflow
description: A valid workflow
inputs:
  target_url:
    type: string
    description: The URL
steps:
  - name: fetch
    action: http.request
    params:
      url: "https://example.com"
`)
	errs := Validate(result)
	assertNoErrors(t, errs)
}

func TestValidate_MissingName(t *testing.T) {
	result := mustParse(t, `
steps:
  - name: fetch
    action: http.request
`)
	errs := Validate(result)
	assertHasError(t, errs, "name")
}

func TestValidate_InvalidNameFormat(t *testing.T) {
	result := mustParse(t, `
name: My Workflow!
steps:
  - name: fetch
    action: http.request
`)
	errs := Validate(result)
	assertHasError(t, errs, "name")
}

func TestValidate_MissingSteps(t *testing.T) {
	result := mustParse(t, `
name: my-workflow
`)
	errs := Validate(result)
	assertHasError(t, errs, "steps")
}

func TestValidate_EmptySteps(t *testing.T) {
	result := mustParse(t, `
name: my-workflow
steps: []
`)
	errs := Validate(result)
	assertHasError(t, errs, "steps")
}

func TestValidate_StepMissingName(t *testing.T) {
	result := mustParse(t, `
name: my-workflow
steps:
  - action: http.request
`)
	errs := Validate(result)
	assertHasError(t, errs, "steps[0].name")
}

func TestValidate_StepInvalidName(t *testing.T) {
	result := mustParse(t, `
name: my-workflow
steps:
  - name: Invalid Step
    action: http.request
`)
	errs := Validate(result)
	assertHasError(t, errs, "steps[0].name")
}

func TestValidate_StepMissingAction(t *testing.T) {
	result := mustParse(t, `
name: my-workflow
steps:
  - name: fetch
`)
	errs := Validate(result)
	assertHasError(t, errs, "steps[0].action")
}

func TestValidate_DuplicateStepNames(t *testing.T) {
	result := mustParse(t, `
name: my-workflow
steps:
  - name: fetch
    action: http.request
  - name: fetch
    action: http.request
`)
	errs := Validate(result)
	assertHasError(t, errs, "steps[1].name")
}

func TestValidate_InvalidInputType(t *testing.T) {
	result := mustParse(t, `
name: my-workflow
inputs:
  count:
    type: integer
steps:
  - name: fetch
    action: http.request
`)
	errs := Validate(result)
	assertHasError(t, errs, "inputs.count.type")
}

func TestValidate_InvalidInputName(t *testing.T) {
	result := mustParse(t, `
name: my-workflow
inputs:
  Bad-Name:
    type: string
steps:
  - name: fetch
    action: http.request
`)
	errs := Validate(result)
	assertHasError(t, errs, "inputs.Bad-Name")
}

func TestValidate_InvalidRetryBackoff(t *testing.T) {
	result := mustParse(t, `
name: my-workflow
steps:
  - name: fetch
    action: http.request
    retry:
      max_attempts: 3
      backoff: linear
`)
	errs := Validate(result)
	assertHasError(t, errs, "steps[0].retry.backoff")
}

func TestValidate_InvalidRetryMaxAttempts(t *testing.T) {
	result := mustParse(t, `
name: my-workflow
steps:
  - name: fetch
    action: http.request
    retry:
      max_attempts: 0
      backoff: fixed
`)
	errs := Validate(result)
	assertHasError(t, errs, "steps[0].retry.max_attempts")
}

func TestValidate_InvalidTimeout(t *testing.T) {
	result := mustParse(t, `
name: my-workflow
steps:
  - name: fetch
    action: http.request
    timeout: "not-a-duration"
`)
	errs := Validate(result)
	assertHasError(t, errs, "steps[0].timeout")
}

func TestValidate_NegativeTimeout(t *testing.T) {
	result := mustParse(t, `
name: my-workflow
steps:
  - name: fetch
    action: http.request
    timeout: "-5s"
`)
	errs := Validate(result)
	assertHasError(t, errs, "steps[0].timeout")
}

func TestValidate_ValidInputTypes(t *testing.T) {
	result := mustParse(t, `
name: my-workflow
inputs:
  name:
    type: string
  count:
    type: number
  enabled:
    type: boolean
steps:
  - name: fetch
    action: http.request
`)
	errs := Validate(result)
	assertNoErrors(t, errs)
}

func TestExtractImplicitDeps(t *testing.T) {
	tests := []struct {
		name string
		step Step
		want []string
	}{
		{
			name: "param references step output",
			step: Step{
				Params: map[string]any{
					"url": "{{ steps.fetch_data.output.url }}",
				},
			},
			want: []string{"fetch_data"},
		},
		{
			name: "if condition references step",
			step: Step{
				If: "steps.check.output.ready == true",
			},
			want: []string{"check"},
		},
		{
			name: "nested param references",
			step: Step{
				Params: map[string]any{
					"body": map[string]any{
						"first":  "{{ steps.a.output.value }}",
						"second": "{{ steps.b.output.value }}",
					},
				},
			},
			want: []string{"a", "b"},
		},
		{
			name: "no references",
			step: Step{
				Params: map[string]any{
					"url": "https://example.com",
				},
			},
			want: nil,
		},
		{
			name: "deduplicates references",
			step: Step{
				If: "steps.x.output.a == steps.x.output.b",
				Params: map[string]any{
					"val": "{{ steps.x.output.c }}",
				},
			},
			want: []string{"x"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractImplicitDeps(tt.step)
			// Sort both for stable comparison.
			sort.Strings(got)
			sort.Strings(tt.want)
			if len(got) != len(tt.want) {
				t.Fatalf("ExtractImplicitDeps() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("ExtractImplicitDeps() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestValidate_UndefinedDependency(t *testing.T) {
	result := mustParse(t, `
name: my-workflow
steps:
  - name: fetch
    action: http.request
    depends_on:
      - nonexistent
`)
	errs := Validate(result)
	assertHasError(t, errs, "steps[0].depends_on")
}

func TestValidate_UndefinedImplicitDependency(t *testing.T) {
	result := mustParse(t, `
name: my-workflow
steps:
  - name: process
    action: http.request
    params:
      url: "{{ steps.missing_step.output.url }}"
`)
	errs := Validate(result)
	assertHasError(t, errs, "steps[0].depends_on")
}

func TestValidate_ValidDependency(t *testing.T) {
	result := mustParse(t, `
name: my-workflow
steps:
  - name: fetch
    action: http.request
  - name: process
    action: http.request
    depends_on:
      - fetch
    params:
      url: "{{ steps.fetch.output.url }}"
`)
	errs := Validate(result)
	assertNoErrors(t, errs)
}

func TestValidate_ToolDuplicateNames(t *testing.T) {
	result := mustParse(t, `
name: my-workflow
steps:
  - name: ask-ai
    action: ai/completion
    params:
      model: gpt-4
      prompt: "do something"
      tools:
        - name: search
          description: Search the web
          input_schema:
            type: object
            properties:
              query:
                type: string
          action: http.request
          params:
            url: "https://search.example.com"
        - name: search
          description: Another search tool
          input_schema:
            type: object
          action: http.request
          params:
            url: "https://other.example.com"
`)
	errs := Validate(result)
	assertErrorContains(t, errs, "duplicate")
}

func TestValidate_ToolMissingDescription(t *testing.T) {
	result := mustParse(t, `
name: my-workflow
steps:
  - name: ask-ai
    action: ai/completion
    params:
      model: gpt-4
      prompt: "do something"
      tools:
        - name: search
          input_schema:
            type: object
          action: http.request
`)
	errs := Validate(result)
	assertErrorContains(t, errs, "description")
}

func TestValidate_ToolMissingInputSchema(t *testing.T) {
	result := mustParse(t, `
name: my-workflow
steps:
  - name: ask-ai
    action: ai/completion
    params:
      model: gpt-4
      prompt: "do something"
      tools:
        - name: search
          description: Search the web
          action: http.request
`)
	errs := Validate(result)
	assertErrorContains(t, errs, "input_schema")
}

func TestValidate_ToolRoundsOutOfBounds(t *testing.T) {
	result := mustParse(t, `
name: my-workflow
steps:
  - name: ask-ai
    action: ai/completion
    params:
      model: gpt-4
      prompt: "do something"
      max_tool_rounds: 100
      tools:
        - name: search
          description: Search the web
          input_schema:
            type: object
          action: http.request
`)
	errs := Validate(result)
	assertErrorContains(t, errs, "max_tool_rounds")
}

func TestValidate_ToolCallsPerRoundOutOfBounds(t *testing.T) {
	result := mustParse(t, `
name: my-workflow
steps:
  - name: ask-ai
    action: ai/completion
    params:
      model: gpt-4
      prompt: "do something"
      max_tool_calls_per_round: 50
      tools:
        - name: search
          description: Search the web
          input_schema:
            type: object
          action: http.request
`)
	errs := Validate(result)
	assertErrorContains(t, errs, "max_tool_calls_per_round")
}

func TestValidate_ValidToolUseStep(t *testing.T) {
	result := mustParse(t, `
name: my-workflow
steps:
  - name: ask-ai
    action: ai/completion
    params:
      model: gpt-4
      prompt: "do something"
      max_tool_rounds: 10
      max_tool_calls_per_round: 5
      tools:
        - name: search
          description: Search the web for information
          input_schema:
            type: object
            properties:
              query:
                type: string
          action: http.request
          params:
            url: "https://search.example.com"
        - name: calculate
          description: Perform a calculation
          input_schema:
            type: object
            properties:
              expression:
                type: string
          action: http.request
          params:
            url: "https://calc.example.com"
`)
	errs := Validate(result)
	assertNoErrors(t, errs)
}

func TestValidate_ArtifactDuplicateNames(t *testing.T) {
	result := mustParse(t, `
name: test-dup-artifacts
description: test
steps:
  - name: step-a
    action: docker/run
    params:
      image: alpine
    artifacts:
      - path: /mantle/artifacts/out.tar.gz
        name: my-artifact
  - name: step-b
    action: docker/run
    params:
      image: alpine
    artifacts:
      - path: /mantle/artifacts/other.tar.gz
        name: my-artifact
`)
	errs := Validate(result)
	if len(errs) == 0 {
		t.Fatal("expected validation error for duplicate artifact name")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "duplicate artifact name") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'duplicate artifact name' error, got: %v", errs)
	}
}

func TestValidate_ArtifactMissingFields(t *testing.T) {
	result := mustParse(t, `
name: test-missing-artifact-fields
description: test
steps:
  - name: step-a
    action: docker/run
    params:
      image: alpine
    artifacts:
      - path: ""
        name: ""
`)
	errs := Validate(result)
	if len(errs) < 2 {
		t.Fatalf("expected at least 2 errors for missing artifact fields, got %d: %v", len(errs), errs)
	}
}

func TestValidate_ArtifactValidDeclaration(t *testing.T) {
	result := mustParse(t, `
name: test-valid-artifacts
description: test
steps:
  - name: step-a
    action: docker/run
    params:
      image: alpine
    artifacts:
      - path: /mantle/artifacts/out.tar.gz
        name: my-artifact
`)
	errs := Validate(result)
	// Filter to artifact-related errors only
	var artifactErrs []ValidationError
	for _, e := range errs {
		if strings.Contains(e.Field, "artifact") {
			artifactErrs = append(artifactErrs, e)
		}
	}
	if len(artifactErrs) > 0 {
		t.Errorf("unexpected artifact validation errors: %v", artifactErrs)
	}
}

func TestValidate_ContinueOnError(t *testing.T) {
	result := mustParse(t, `
name: test-continue
steps:
  - name: risky-step
    action: http/request
    continue_on_error: true
    params:
      url: https://example.com
  - name: notify
    action: slack/send
    if: "steps['risky-step'].error != null"
    params:
      channel: "#alerts"
      text: "Step failed"
`)
	errs := Validate(result)
	assertNoErrors(t, errs)
	if result.Workflow.Steps[0].ContinueOnError != true {
		t.Errorf("expected ContinueOnError to be true for steps[0], got %v", result.Workflow.Steps[0].ContinueOnError)
	}
	if result.Workflow.Steps[1].ContinueOnError != false {
		t.Errorf("expected ContinueOnError to be false for steps[1], got %v", result.Workflow.Steps[1].ContinueOnError)
	}
}
