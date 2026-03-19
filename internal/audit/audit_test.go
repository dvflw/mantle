package audit

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"
)

func TestNoopEmitter(t *testing.T) {
	var emitter Emitter = &NoopEmitter{}

	err := emitter.Emit(context.Background(), Event{
		Action:   ActionWorkflowApplied,
		Resource: Resource{Type: "workflow_definition", ID: "test"},
	})
	if err != nil {
		t.Fatalf("NoopEmitter.Emit() returned error: %v", err)
	}
}

func TestLogEmitter(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	var emitter Emitter = &LogEmitter{Logger: logger}

	err := emitter.Emit(context.Background(), Event{
		ID:        "evt-1",
		Timestamp: time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC),
		Actor:     "cli",
		Action:    ActionWorkflowApplied,
		Resource:  Resource{Type: "workflow_definition", ID: "my-workflow"},
		Metadata:  map[string]string{"version": "3"},
	})
	if err != nil {
		t.Fatalf("LogEmitter.Emit() returned error: %v", err)
	}

	output := buf.String()
	for _, want := range []string{"workflow.applied", "workflow_definition", "my-workflow", "cli"} {
		if !bytes.Contains([]byte(output), []byte(want)) {
			t.Errorf("LogEmitter output missing %q, got: %s", want, output)
		}
	}
}

func TestLogEmitter_MinimalEvent(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	emitter := &LogEmitter{Logger: logger}
	err := emitter.Emit(context.Background(), Event{
		Action:   ActionStepCompleted,
		Resource: Resource{Type: "step_execution", ID: "step-1"},
	})
	if err != nil {
		t.Fatalf("LogEmitter.Emit() returned error: %v", err)
	}

	if buf.Len() == 0 {
		t.Error("LogEmitter produced no output for minimal event")
	}
}

func TestLogEmitter_DefaultLogger(t *testing.T) {
	emitter := &LogEmitter{} // nil logger — should use slog.Default()
	err := emitter.Emit(context.Background(), Event{
		Action:   ActionStepStarted,
		Resource: Resource{Type: "step_execution", ID: "step-1"},
	})
	if err != nil {
		t.Fatalf("LogEmitter.Emit() with nil logger returned error: %v", err)
	}
}
