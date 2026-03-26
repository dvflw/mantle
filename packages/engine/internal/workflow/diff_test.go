package workflow

import (
	"strings"
	"testing"
)

func TestDiff_NewWorkflow(t *testing.T) {
	wf := &Workflow{
		Name:        "my-workflow",
		Description: "A test workflow",
		Steps: []Step{
			{Name: "step-one", Action: "http/request"},
		},
	}

	result := Diff(nil, wf)
	if !strings.Contains(result, "+ new workflow: my-workflow") {
		t.Errorf("expected new workflow header, got:\n%s", result)
	}
	if !strings.Contains(result, `+ step "step-one"`) {
		t.Errorf("expected step addition, got:\n%s", result)
	}
}

func TestDiff_NoChanges(t *testing.T) {
	wf := &Workflow{
		Name:  "my-workflow",
		Steps: []Step{{Name: "s1", Action: "http/request"}},
	}
	result := Diff(wf, wf)
	if result != "" {
		t.Errorf("expected empty diff, got:\n%s", result)
	}
}

func TestDiff_DescriptionChanged(t *testing.T) {
	old := &Workflow{Name: "wf", Description: "old desc"}
	new := &Workflow{Name: "wf", Description: "new desc"}
	result := Diff(old, new)
	if !strings.Contains(result, `~ description: "old desc" → "new desc"`) {
		t.Errorf("expected description change, got:\n%s", result)
	}
}

func TestDiff_StepAdded(t *testing.T) {
	old := &Workflow{Name: "wf", Steps: []Step{{Name: "s1", Action: "http/request"}}}
	new := &Workflow{Name: "wf", Steps: []Step{
		{Name: "s1", Action: "http/request"},
		{Name: "s2", Action: "http/request"},
	}}
	result := Diff(old, new)
	if !strings.Contains(result, `+ step "s2"`) {
		t.Errorf("expected step addition, got:\n%s", result)
	}
}

func TestDiff_StepRemoved(t *testing.T) {
	old := &Workflow{Name: "wf", Steps: []Step{
		{Name: "s1", Action: "http/request"},
		{Name: "s2", Action: "http/request"},
	}}
	new := &Workflow{Name: "wf", Steps: []Step{{Name: "s1", Action: "http/request"}}}
	result := Diff(old, new)
	if !strings.Contains(result, `- step "s2"`) {
		t.Errorf("expected step removal, got:\n%s", result)
	}
}

func TestDiff_StepModified(t *testing.T) {
	old := &Workflow{Name: "wf", Steps: []Step{{Name: "s1", Action: "http/request", Timeout: "10s"}}}
	new := &Workflow{Name: "wf", Steps: []Step{{Name: "s1", Action: "http/request", Timeout: "30s"}}}
	result := Diff(old, new)
	if !strings.Contains(result, `~ step "s1"`) {
		t.Errorf("expected step modification, got:\n%s", result)
	}
	if !strings.Contains(result, `timeout: "10s" → "30s"`) {
		t.Errorf("expected timeout change, got:\n%s", result)
	}
}

func TestDiff_InputAdded(t *testing.T) {
	old := &Workflow{Name: "wf", Inputs: map[string]Input{}}
	new := &Workflow{Name: "wf", Inputs: map[string]Input{"url": {Type: "string"}}}
	result := Diff(old, new)
	if !strings.Contains(result, `+ input "url"`) {
		t.Errorf("expected input addition, got:\n%s", result)
	}
}

func TestDiff_InputRemoved(t *testing.T) {
	old := &Workflow{Name: "wf", Inputs: map[string]Input{"url": {Type: "string"}}}
	new := &Workflow{Name: "wf", Inputs: map[string]Input{}}
	result := Diff(old, new)
	if !strings.Contains(result, `- input "url"`) {
		t.Errorf("expected input removal, got:\n%s", result)
	}
}
