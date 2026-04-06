package cel

import (
	"testing"
)

func TestSetValuesEnv_LayersBetweenConfigAndOS(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error: %v", err)
	}

	eval.SetConfigEnv(map[string]string{
		"LOG_LEVEL": "info",
		"REGION":    "us-west-2",
		"APP_NAME":  "config-app",
	})

	eval.SetValuesEnv(map[string]string{
		"LOG_LEVEL": "error",
		"DB_HOST":   "prod-db.example.com",
	})

	ctx := &Context{
		Steps:  make(map[string]map[string]any),
		Inputs: make(map[string]any),
	}

	// Values override config.
	result, err := eval.Eval(`env.LOG_LEVEL`, ctx)
	if err != nil {
		t.Fatalf("Eval(env.LOG_LEVEL) error: %v", err)
	}
	if result != "error" {
		t.Errorf("env.LOG_LEVEL = %v, want %q (values overrides config)", result, "error")
	}

	// Config value preserved when values doesn't override.
	result, err = eval.Eval(`env.REGION`, ctx)
	if err != nil {
		t.Fatalf("Eval(env.REGION) error: %v", err)
	}
	if result != "us-west-2" {
		t.Errorf("env.REGION = %v, want %q (config preserved)", result, "us-west-2")
	}

	// Values-only key is accessible.
	result, err = eval.Eval(`env.DB_HOST`, ctx)
	if err != nil {
		t.Fatalf("Eval(env.DB_HOST) error: %v", err)
	}
	if result != "prod-db.example.com" {
		t.Errorf("env.DB_HOST = %v, want %q", result, "prod-db.example.com")
	}
}

func TestSetValuesEnv_OSEnvTakesPrecedence(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error: %v", err)
	}

	t.Setenv("MANTLE_ENV_LOG_LEVEL", "debug")

	eval.SetConfigEnv(map[string]string{
		"LOG_LEVEL": "info",
	})
	eval.SetValuesEnv(map[string]string{
		"LOG_LEVEL": "error",
	})

	ctx := &Context{
		Steps:  make(map[string]map[string]any),
		Inputs: make(map[string]any),
	}

	result, err := eval.Eval(`env.LOG_LEVEL`, ctx)
	if err != nil {
		t.Fatalf("Eval(env.LOG_LEVEL) error: %v", err)
	}
	if result != "debug" {
		t.Errorf("env.LOG_LEVEL = %v, want %q (OS env wins)", result, "debug")
	}
}

func TestSetValuesEnv_NilClearsValuesLayer(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error: %v", err)
	}

	eval.SetConfigEnv(map[string]string{
		"LOG_LEVEL": "info",
	})
	eval.SetValuesEnv(map[string]string{
		"LOG_LEVEL": "error",
	})

	eval.SetValuesEnv(nil)

	ctx := &Context{
		Steps:  make(map[string]map[string]any),
		Inputs: make(map[string]any),
	}

	result, err := eval.Eval(`env.LOG_LEVEL`, ctx)
	if err != nil {
		t.Fatalf("Eval(env.LOG_LEVEL) error: %v", err)
	}
	if result != "info" {
		t.Errorf("env.LOG_LEVEL = %v, want %q (back to config after clear)", result, "info")
	}
}
