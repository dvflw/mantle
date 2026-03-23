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

func TestEnrichFromContext_WithExtractor(t *testing.T) {
	p := &PostgresEmitter{
		AuthMethodExtractor: func(ctx context.Context) string {
			return "api_key"
		},
	}

	event := Event{
		Action:   ActionWorkflowApplied,
		Resource: Resource{Type: "workflow_definition", ID: "wf-1"},
	}

	enriched := p.enrichFromContext(context.Background(), event)

	if enriched.Metadata == nil {
		t.Fatal("expected Metadata to be non-nil")
	}
	if enriched.Metadata["auth_method"] != "api_key" {
		t.Errorf("auth_method = %q, want api_key", enriched.Metadata["auth_method"])
	}
}

func TestEnrichFromContext_NilExtractor(t *testing.T) {
	p := &PostgresEmitter{
		AuthMethodExtractor: nil,
	}

	event := Event{
		Action:   ActionStepStarted,
		Resource: Resource{Type: "step_execution", ID: "s-1"},
		Metadata: map[string]string{"existing": "value"},
	}

	enriched := p.enrichFromContext(context.Background(), event)

	if enriched.Metadata["existing"] != "value" {
		t.Errorf("existing metadata should be preserved, got %v", enriched.Metadata)
	}
	if _, ok := enriched.Metadata["auth_method"]; ok {
		t.Error("auth_method should not be set when extractor is nil")
	}
}

func TestEnrichFromContext_EmptyMethod(t *testing.T) {
	p := &PostgresEmitter{
		AuthMethodExtractor: func(ctx context.Context) string {
			return ""
		},
	}

	event := Event{
		Action:   ActionStepCompleted,
		Resource: Resource{Type: "step_execution", ID: "s-2"},
	}

	enriched := p.enrichFromContext(context.Background(), event)

	if enriched.Metadata != nil {
		if _, ok := enriched.Metadata["auth_method"]; ok {
			t.Error("auth_method should not be added when method is empty")
		}
	}
}

func TestMarshalNullableJSON_Nil(t *testing.T) {
	b, err := marshalNullableJSON(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b != nil {
		t.Errorf("expected nil, got %v", b)
	}
}

func TestMarshalNullableJSON_Value(t *testing.T) {
	b, err := marshalNullableJSON(map[string]string{"key": "val"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil bytes")
	}
	if string(b) != `{"key":"val"}` {
		t.Errorf("got %s, want {\"key\":\"val\"}", string(b))
	}
}

func TestNullableBytes_Nil(t *testing.T) {
	result := nullableBytes(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestNullableBytes_Value(t *testing.T) {
	input := []byte(`{"hello":"world"}`)
	result := nullableBytes(input)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	b, ok := result.([]byte)
	if !ok {
		t.Fatalf("expected []byte, got %T", result)
	}
	if string(b) != string(input) {
		t.Errorf("got %s, want %s", string(b), string(input))
	}
}
