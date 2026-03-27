package workflow

import (
	"strings"
	"testing"
)

func TestValidate_HooksNoDependsOn(t *testing.T) {
	result := mustParse(t, `
name: my-workflow
steps:
  - name: do-work
    action: http/request
    params:
      url: "https://example.com"
hooks:
  on_success:
    - name: notify
      action: slack/send
      depends_on:
        - do-work
      params:
        channel: "#alerts"
        text: "done"
`)
	errs := Validate(result)
	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "hook steps do not support depends_on") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error about hook steps not supporting depends_on, got: %v", errs)
	}
}

func TestValidate_HookStepNameUniqueness(t *testing.T) {
	result := mustParse(t, `
name: my-workflow
steps:
  - name: do-work
    action: http/request
    params:
      url: "https://example.com"
hooks:
  on_failure:
    - name: notify
      action: slack/send
      params:
        channel: "#alerts"
        text: "failed"
    - name: notify
      action: email/send
      params:
        to: "admin@example.com"
        body: "failed"
`)
	errs := Validate(result)
	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "duplicate") && strings.Contains(e.Message, "notify") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected duplicate name error for hook step, got: %v", errs)
	}
}

func TestValidate_ConcurrencyFields(t *testing.T) {
	result := mustParse(t, `
name: my-workflow
max_parallel_executions: 5
on_limit: queue
steps:
  - name: do-work
    action: http/request
    max_parallel: 3
    params:
      url: "https://example.com"
`)
	errs := Validate(result)
	assertNoErrors(t, errs)
	if result.Workflow.MaxParallelExecutions != 5 {
		t.Errorf("expected MaxParallelExecutions=5, got %d", result.Workflow.MaxParallelExecutions)
	}
	if result.Workflow.OnLimit != "queue" {
		t.Errorf("expected OnLimit=%q, got %q", "queue", result.Workflow.OnLimit)
	}
	if result.Workflow.Steps[0].MaxParallel != 3 {
		t.Errorf("expected MaxParallel=3, got %d", result.Workflow.Steps[0].MaxParallel)
	}
}

func TestValidate_InvalidOnLimit(t *testing.T) {
	result := mustParse(t, `
name: my-workflow
on_limit: drop
steps:
  - name: do-work
    action: http/request
    params:
      url: "https://example.com"
`)
	errs := Validate(result)
	assertHasError(t, errs, "on_limit")
}

func TestValidate_HooksTimeout(t *testing.T) {
	t.Run("valid timeout", func(t *testing.T) {
		result := mustParse(t, `
name: my-workflow
steps:
  - name: do-work
    action: http/request
    params:
      url: "https://example.com"
hooks:
  timeout: 5m
  on_success:
    - name: notify
      action: slack/send
      params:
        channel: "#alerts"
        text: "done"
`)
		errs := Validate(result)
		assertNoErrors(t, errs)
	})

	t.Run("invalid timeout", func(t *testing.T) {
		result := mustParse(t, `
name: my-workflow
steps:
  - name: do-work
    action: http/request
    params:
      url: "https://example.com"
hooks:
  timeout: not-a-duration
  on_success:
    - name: notify
      action: slack/send
      params:
        channel: "#alerts"
        text: "done"
`)
		errs := Validate(result)
		assertHasError(t, errs, "hooks.timeout")
	})
}

func TestValidate_HookStepMissingName(t *testing.T) {
	result := mustParse(t, `
name: my-workflow
steps:
  - name: do-work
    action: http/request
    params:
      url: "https://example.com"
hooks:
  on_success:
    - action: slack/send
      params:
        channel: "#alerts"
`)
	errs := Validate(result)
	assertHasError(t, errs, "hooks.on_success[0].name")
}

func TestValidate_HookStepMissingAction(t *testing.T) {
	result := mustParse(t, `
name: my-workflow
steps:
  - name: do-work
    action: http/request
    params:
      url: "https://example.com"
hooks:
  on_finish:
    - name: cleanup
      params:
        channel: "#alerts"
`)
	errs := Validate(result)
	assertHasError(t, errs, "hooks.on_finish[0].action")
}

func TestValidate_NegativeMaxParallelExecutions(t *testing.T) {
	result := mustParse(t, `
name: my-workflow
max_parallel_executions: -1
steps:
  - name: do-work
    action: http/request
    params:
      url: "https://example.com"
`)
	errs := Validate(result)
	assertHasError(t, errs, "max_parallel_executions")
}

func TestValidate_NegativeStepMaxParallel(t *testing.T) {
	result := mustParse(t, `
name: my-workflow
steps:
  - name: do-work
    action: http/request
    max_parallel: -1
    params:
      url: "https://example.com"
`)
	errs := Validate(result)
	assertHasError(t, errs, "steps[0].max_parallel")
}

func TestValidate_OnLimitReject(t *testing.T) {
	result := mustParse(t, `
name: my-workflow
on_limit: reject
steps:
  - name: do-work
    action: http/request
    params:
      url: "https://example.com"
`)
	errs := Validate(result)
	assertNoErrors(t, errs)
}

func TestValidate_HookStepNameUniqueAcrossBlocks(t *testing.T) {
	// Same name in different hook blocks is OK (each block has its own namespace)
	result := mustParse(t, `
name: my-workflow
steps:
  - name: do-work
    action: http/request
    params:
      url: "https://example.com"
hooks:
  on_success:
    - name: notify
      action: slack/send
      params:
        text: "success"
  on_failure:
    - name: notify
      action: slack/send
      params:
        text: "failure"
`)
	errs := Validate(result)
	assertNoErrors(t, errs)
}
