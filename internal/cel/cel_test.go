package cel

import (
	"testing"
)

func newTestContext() *Context {
	return &Context{
		Steps: map[string]map[string]any{
			"fetch": {
				"output": map[string]any{
					"status": int64(200),
					"body":   "hello world",
					"headers": map[string]any{
						"content-type": "application/json",
					},
				},
			},
		},
		Inputs: map[string]any{
			"url":     "https://example.com",
			"verbose": true,
			"count":   int64(3),
		},
	}
}

func TestEval_StepOutput(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error: %v", err)
	}

	ctx := newTestContext()

	result, err := eval.Eval(`steps.fetch.output.body`, ctx)
	if err != nil {
		t.Fatalf("Eval() error: %v", err)
	}
	if result != "hello world" {
		t.Errorf("Eval() = %v, want %q", result, "hello world")
	}
}

func TestEval_Inputs(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error: %v", err)
	}

	ctx := newTestContext()

	result, err := eval.Eval(`inputs.url`, ctx)
	if err != nil {
		t.Fatalf("Eval() error: %v", err)
	}
	if result != "https://example.com" {
		t.Errorf("Eval() = %v, want %q", result, "https://example.com")
	}
}

func TestEval_Arithmetic(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error: %v", err)
	}

	ctx := newTestContext()

	result, err := eval.Eval(`inputs.count + 1`, ctx)
	if err != nil {
		t.Fatalf("Eval() error: %v", err)
	}
	if result != int64(4) {
		t.Errorf("Eval() = %v (%T), want 4", result, result)
	}
}

func TestEvalBool_True(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error: %v", err)
	}

	ctx := newTestContext()

	result, err := eval.EvalBool(`steps.fetch.output.status == 200`, ctx)
	if err != nil {
		t.Fatalf("EvalBool() error: %v", err)
	}
	if !result {
		t.Error("EvalBool() = false, want true")
	}
}

func TestEvalBool_False(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error: %v", err)
	}

	ctx := newTestContext()

	result, err := eval.EvalBool(`steps.fetch.output.status == 404`, ctx)
	if err != nil {
		t.Fatalf("EvalBool() error: %v", err)
	}
	if result {
		t.Error("EvalBool() = true, want false")
	}
}

func TestEvalBool_InvalidType(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error: %v", err)
	}

	ctx := newTestContext()

	_, err = eval.EvalBool(`inputs.url`, ctx)
	if err == nil {
		t.Error("EvalBool() expected error for non-bool expression, got nil")
	}
}

func TestEval_CompileError(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error: %v", err)
	}

	ctx := newTestContext()

	_, err = eval.Eval(`this is not valid CEL!!!`, ctx)
	if err == nil {
		t.Error("Eval() expected compile error, got nil")
	}
}

func TestResolveString_SingleExpression(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error: %v", err)
	}

	ctx := newTestContext()

	result, err := eval.ResolveString(`{{ inputs.url }}`, ctx)
	if err != nil {
		t.Fatalf("ResolveString() error: %v", err)
	}
	if result != "https://example.com" {
		t.Errorf("ResolveString() = %v, want %q", result, "https://example.com")
	}
}

func TestResolveString_Interpolation(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error: %v", err)
	}

	ctx := newTestContext()

	result, err := eval.ResolveString(`Status: {{ steps.fetch.output.status }}`, ctx)
	if err != nil {
		t.Fatalf("ResolveString() error: %v", err)
	}
	if result != "Status: 200" {
		t.Errorf("ResolveString() = %v, want %q", result, "Status: 200")
	}
}

func TestResolveString_NoExpression(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error: %v", err)
	}

	ctx := newTestContext()

	result, err := eval.ResolveString(`plain string`, ctx)
	if err != nil {
		t.Fatalf("ResolveString() error: %v", err)
	}
	if result != "plain string" {
		t.Errorf("ResolveString() = %v, want %q", result, "plain string")
	}
}

func TestResolveString_PreservesType(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error: %v", err)
	}

	ctx := newTestContext()

	// A single {{ expr }} that returns an int should preserve the int type.
	result, err := eval.ResolveString(`{{ steps.fetch.output.status }}`, ctx)
	if err != nil {
		t.Fatalf("ResolveString() error: %v", err)
	}
	if _, ok := result.(int64); !ok {
		t.Errorf("ResolveString() type = %T, want int64", result)
	}
}

func TestResolveParams(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error: %v", err)
	}

	ctx := newTestContext()

	params := map[string]any{
		"url":    "{{ inputs.url }}",
		"method": "GET",
		"nested": map[string]any{
			"body": "{{ steps.fetch.output.body }}",
		},
	}

	resolved, err := eval.ResolveParams(params, ctx)
	if err != nil {
		t.Fatalf("ResolveParams() error: %v", err)
	}

	if resolved["url"] != "https://example.com" {
		t.Errorf("url = %v, want %q", resolved["url"], "https://example.com")
	}
	if resolved["method"] != "GET" {
		t.Errorf("method = %v, want %q", resolved["method"], "GET")
	}
	nested, ok := resolved["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested = %T, want map[string]any", resolved["nested"])
	}
	if nested["body"] != "hello world" {
		t.Errorf("nested.body = %v, want %q", nested["body"], "hello world")
	}
}

func TestEvaluator_ToolInput(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error: %v", err)
	}

	ctx := &Context{
		Steps:  map[string]map[string]any{},
		Inputs: map[string]any{},
		ToolInput: map[string]any{
			"city":  "London",
			"units": "celsius",
		},
	}

	city, err := eval.ResolveString(`{{ tool_input.city }}`, ctx)
	if err != nil {
		t.Fatalf("ResolveString(tool_input.city) error: %v", err)
	}
	if city != "London" {
		t.Errorf("tool_input.city = %v, want %q", city, "London")
	}

	units, err := eval.ResolveString(`{{ tool_input.units }}`, ctx)
	if err != nil {
		t.Fatalf("ResolveString(tool_input.units) error: %v", err)
	}
	if units != "celsius" {
		t.Errorf("tool_input.units = %v, want %q", units, "celsius")
	}
}

func TestEvaluator_ToolInputNil(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error: %v", err)
	}

	ctx := newTestContext() // ToolInput is nil

	result, err := eval.ResolveString(`{{ inputs.url }}`, ctx)
	if err != nil {
		t.Fatalf("ResolveString() error: %v", err)
	}
	if result != "https://example.com" {
		t.Errorf("ResolveString() = %v, want %q", result, "https://example.com")
	}
}

func TestEvaluator_ToolInputInParams(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error: %v", err)
	}

	ctx := &Context{
		Steps:  map[string]map[string]any{},
		Inputs: map[string]any{},
		ToolInput: map[string]any{
			"city":  "London",
			"units": "celsius",
		},
	}

	params := map[string]any{
		"url":    "https://api.weather.com/{{ tool_input.city }}",
		"method": "GET",
		"query": map[string]any{
			"units": "{{ tool_input.units }}",
		},
	}

	resolved, err := eval.ResolveParams(params, ctx)
	if err != nil {
		t.Fatalf("ResolveParams() error: %v", err)
	}

	if resolved["url"] != "https://api.weather.com/London" {
		t.Errorf("url = %v, want %q", resolved["url"], "https://api.weather.com/London")
	}
	if resolved["method"] != "GET" {
		t.Errorf("method = %v, want %q", resolved["method"], "GET")
	}
	query, ok := resolved["query"].(map[string]any)
	if !ok {
		t.Fatalf("query = %T, want map[string]any", resolved["query"])
	}
	if query["units"] != "celsius" {
		t.Errorf("query.units = %v, want %q", query["units"], "celsius")
	}
}
