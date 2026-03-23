package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dvflw/mantle/internal/engine"
)

func TestStatusIcon(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"completed", "✓"},
		{"failed", "✗"},
		{"running", "▶"},
		{"skipped", "⊘"},
		{"cancelled", "■"},
		{"pending", "○"},
		{"unknown", "○"},
		{"", "○"},
	}
	for _, tc := range tests {
		got := statusIcon(tc.status)
		if got != tc.want {
			t.Errorf("statusIcon(%q) = %q, want %q", tc.status, got, tc.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0ms"},
		{500 * time.Millisecond, "500ms"},
		{999 * time.Millisecond, "999ms"},
		{1 * time.Second, "1.0s"},
		{3200 * time.Millisecond, "3.2s"},
		{59 * time.Second, "59.0s"},
		{60 * time.Second, "1.0m"},
		{90 * time.Second, "1.5m"},
		{5 * time.Minute, "5.0m"},
	}
	for _, tc := range tests {
		got := formatDuration(tc.d)
		if got != tc.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 8, "hello..."},
		{"abcdef", 5, "ab..."},
		{"", 5, ""},
		{"abc", 3, "abc"},
		{"abcd", 3, "..."},
	}
	for _, tc := range tests {
		got := truncate(tc.input, tc.maxLen)
		if got != tc.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tc.input, tc.maxLen, got, tc.want)
		}
	}
}

func TestOrderedSteps(t *testing.T) {
	result := &engine.ExecutionResult{
		Steps: map[string]engine.StepResult{
			"charlie": {Status: "completed", Duration: 1 * time.Second},
			"alpha":   {Status: "failed", Duration: 2 * time.Second},
			"bravo":   {Status: "skipped", Duration: 0},
		},
	}

	steps := orderedSteps(result)

	if len(steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(steps))
	}

	// Should be sorted alphabetically by name.
	expectedOrder := []string{"alpha", "bravo", "charlie"}
	for i, want := range expectedOrder {
		if steps[i].name != want {
			t.Errorf("step[%d].name = %q, want %q", i, steps[i].name, want)
		}
	}

	// Check statuses are preserved.
	if steps[0].status != "failed" {
		t.Errorf("alpha status = %q, want 'failed'", steps[0].status)
	}
	if steps[1].status != "skipped" {
		t.Errorf("bravo status = %q, want 'skipped'", steps[1].status)
	}
	if steps[2].status != "completed" {
		t.Errorf("charlie status = %q, want 'completed'", steps[2].status)
	}
}

func TestOrderedSteps_WithOutput(t *testing.T) {
	result := &engine.ExecutionResult{
		Steps: map[string]engine.StepResult{
			"step1": {
				Status: "completed",
				Output: map[string]any{"key": "value"},
			},
		},
	}

	steps := orderedSteps(result)
	if len(steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(steps))
	}
	if steps[0].output == "" {
		t.Error("expected non-empty output for step with Output map")
	}
	if !strings.Contains(steps[0].output, "value") {
		t.Errorf("output %q should contain 'value'", steps[0].output)
	}
}

func TestOrderedSteps_Empty(t *testing.T) {
	result := &engine.ExecutionResult{
		Steps: map[string]engine.StepResult{},
	}
	steps := orderedSteps(result)
	if len(steps) != 0 {
		t.Errorf("expected 0 steps, got %d", len(steps))
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input   string
		want    time.Duration
		wantErr bool
	}{
		{"1h", 1 * time.Hour, false},
		{"24h", 24 * time.Hour, false},
		{"30m", 30 * time.Minute, false},
		{"7d", 7 * 24 * time.Hour, false},
		{"1d", 24 * time.Hour, false},
		{"30d", 30 * 24 * time.Hour, false},
		{"-1h", 0, true},
		{"0d", 0, true},
		{"-1d", 0, true},
		{"abc", 0, true},
		{"xd", 0, true},
	}
	for _, tc := range tests {
		got, err := parseDuration(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseDuration(%q) expected error, got %v", tc.input, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseDuration(%q) unexpected error: %v", tc.input, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseDuration(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

// --- Validate command tests ---

func TestValidateCommand_ValidFile(t *testing.T) {
	cmd := NewRootCommand()
	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"validate", "../../examples/hello-world.yaml"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("validate valid file returned error: %v\nstderr: %s", err, errBuf.String())
	}

	output := buf.String()
	if !strings.Contains(output, "valid") {
		t.Errorf("expected 'valid' in output, got %q", output)
	}
}

func TestValidateCommand_InvalidFile(t *testing.T) {
	// Create a malformed YAML file.
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("not: a: valid: workflow\n  broken"), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCommand()
	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"validate", path})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestValidateCommand_FileNotFound(t *testing.T) {
	cmd := NewRootCommand()
	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"validate", "/nonexistent/workflow.yaml"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

func TestValidateCommand_MissingSteps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "no-steps.yaml")
	content := "name: test-workflow\ndescription: missing steps\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCommand()
	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"validate", path})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for workflow with no steps, got nil")
	}
}
