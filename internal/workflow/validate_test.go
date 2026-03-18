package workflow

import "testing"

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
