package cel

import (
	"os"
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

func TestEval_EnvVarFiltering(t *testing.T) {
	// Set a safe MANTLE_ENV_ variable that should be accessible.
	t.Setenv("MANTLE_ENV_APP_MODE", "production")

	// Set variables that must NOT be accessible via CEL.
	t.Setenv("MANTLE_ENCRYPTION_KEY", "super-secret-key")
	t.Setenv("MANTLE_DATABASE_URL", "postgres://localhost/mantle")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "aws-secret")

	// Evaluator must be created after Setenv so the cache picks them up.
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error: %v", err)
	}

	ctx := newTestContext()

	// MANTLE_ENV_APP_MODE should be accessible as env.APP_MODE (prefix stripped).
	result, err := eval.Eval(`env.APP_MODE`, ctx)
	if err != nil {
		t.Fatalf("Eval(env.APP_MODE) error: %v", err)
	}
	if result != "production" {
		t.Errorf("env.APP_MODE = %v, want %q", result, "production")
	}
}

func TestEval_EnvVarBlocksSensitive(t *testing.T) {
	t.Setenv("MANTLE_ENCRYPTION_KEY", "super-secret-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "aws-secret")

	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error: %v", err)
	}

	ctx := newTestContext()

	// These must not be accessible — they lack the MANTLE_ENV_ prefix.
	// Accessing a missing key in a CEL map produces an evaluation error.
	_, err = eval.Eval(`env.MANTLE_ENCRYPTION_KEY`, ctx)
	if err == nil {
		t.Error("expected error accessing env.MANTLE_ENCRYPTION_KEY, got nil")
	}

	_, err = eval.Eval(`env.AWS_SECRET_ACCESS_KEY`, ctx)
	if err == nil {
		t.Error("expected error accessing env.AWS_SECRET_ACCESS_KEY, got nil")
	}
}

func TestEnvVars_PrefixStripping(t *testing.T) {
	// Directly test the envVars function.
	os.Setenv("MANTLE_ENV_TEST_KEY", "test-value")
	defer os.Unsetenv("MANTLE_ENV_TEST_KEY")

	os.Setenv("MANTLE_DATABASE_URL", "should-not-appear")
	defer os.Unsetenv("MANTLE_DATABASE_URL")

	result := envVars()

	if v, ok := result["TEST_KEY"]; !ok || v != "test-value" {
		t.Errorf("envVars()[TEST_KEY] = %q, %v; want %q, true", v, ok, "test-value")
	}

	if _, ok := result["MANTLE_DATABASE_URL"]; ok {
		t.Error("envVars() should not contain MANTLE_DATABASE_URL")
	}

	if _, ok := result["DATABASE_URL"]; ok {
		t.Error("envVars() should not contain DATABASE_URL (MANTLE_ prefix without ENV_)")
	}
}

func TestEval_HookAndExecutionVariables(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error: %v", err)
	}

	ctx := &Context{
		Steps:  map[string]map[string]any{},
		Inputs: map[string]any{},
		Hooks: map[string]map[string]any{
			"notify": {
				"output": map[string]any{
					"sent": true,
				},
			},
		},
		Execution: map[string]any{
			"status":      "failed",
			"failed_step": "fetch",
		},
	}

	// Test: hooks['notify'].output.sent == true
	result, err := eval.EvalBool(`hooks['notify'].output.sent == true`, ctx)
	if err != nil {
		t.Fatalf("Eval hooks sent: %v", err)
	}
	if !result {
		t.Error("hooks['notify'].output.sent should be true")
	}

	// Test: execution.status == "failed"
	result, err = eval.EvalBool(`execution.status == "failed"`, ctx)
	if err != nil {
		t.Fatalf("Eval execution.status: %v", err)
	}
	if !result {
		t.Error("execution.status should be 'failed'")
	}

	// Test: execution.failed_step == "fetch"
	result, err = eval.EvalBool(`execution.failed_step == "fetch"`, ctx)
	if err != nil {
		t.Fatalf("Eval execution.failed_step: %v", err)
	}
	if !result {
		t.Error("execution.failed_step should be 'fetch'")
	}
}

func TestEval_ArtifactsAccess(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatal(err)
	}

	ctx := &Context{
		Steps:  map[string]map[string]any{},
		Inputs: map[string]any{},
		Artifacts: map[string]map[string]any{
			"backup-archive": {
				"name": "backup-archive",
				"url":  "s3://bucket/path/backup.tar.gz",
				"size": int64(1048576),
			},
		},
	}

	// Test accessing artifact URL
	result, err := eval.Eval("artifacts['backup-archive'].url", ctx)
	if err != nil {
		t.Fatalf("Eval url: %v", err)
	}
	if result != "s3://bucket/path/backup.tar.gz" {
		t.Errorf("url result = %v, want s3 URL", result)
	}

	// Test accessing artifact size
	result, err = eval.Eval("artifacts['backup-archive'].size", ctx)
	if err != nil {
		t.Fatalf("Eval size: %v", err)
	}
	if result != int64(1048576) {
		t.Errorf("size result = %v, want 1048576", result)
	}
}
