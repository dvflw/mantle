package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeValuesFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "values.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write values file: %v", err)
	}
	return path
}

// --- LoadValues / LoadValuesBytes tests ---

func TestLoadValues_ValidInputsAndEnv(t *testing.T) {
	path := writeValuesFile(t, `
inputs:
  url: https://example.com
  count: 42
env:
  API_KEY: secret123
  BASE_URL: https://api.example.com
`)

	vals, err := LoadValues(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if vals.Inputs["url"] != "https://example.com" {
		t.Errorf("inputs.url: got %v, want %q", vals.Inputs["url"], "https://example.com")
	}
	if vals.Inputs["count"] != 42 {
		t.Errorf("inputs.count: got %v, want 42", vals.Inputs["count"])
	}
	if vals.Env["API_KEY"] != "secret123" {
		t.Errorf("env.API_KEY: got %q, want %q", vals.Env["API_KEY"], "secret123")
	}
	if vals.Env["BASE_URL"] != "https://api.example.com" {
		t.Errorf("env.BASE_URL: got %q, want %q", vals.Env["BASE_URL"], "https://api.example.com")
	}
}

func TestLoadValues_FileNotFound(t *testing.T) {
	_, err := LoadValues("/nonexistent/path/values.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !strings.Contains(err.Error(), "reading values file") {
		t.Errorf("error message should mention reading: %v", err)
	}
}

func TestLoadValues_InvalidYAML(t *testing.T) {
	path := writeValuesFile(t, `
inputs: [
  - broken: yaml: here
`)

	_, err := LoadValues(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
	if !strings.Contains(err.Error(), "parsing values file") {
		t.Errorf("error message should mention parsing: %v", err)
	}
}

func TestLoadValues_UnknownKey(t *testing.T) {
	path := writeValuesFile(t, `
inputs:
  foo: bar
secrets:
  api_key: mysecret
`)

	_, err := LoadValues(path)
	if err == nil {
		t.Fatal("expected error for unknown key, got nil")
	}
	if !strings.Contains(err.Error(), "unknown key") {
		t.Errorf("error should mention unknown key: %v", err)
	}
	if !strings.Contains(err.Error(), `"secrets"`) {
		t.Errorf("error should name the offending key: %v", err)
	}
}

func TestLoadValues_InputsOnly(t *testing.T) {
	path := writeValuesFile(t, `
inputs:
  timeout: 30s
  retries: 3
`)

	vals, err := LoadValues(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(vals.Inputs) != 2 {
		t.Errorf("inputs: got %d, want 2", len(vals.Inputs))
	}
	if vals.Inputs["timeout"] != "30s" {
		t.Errorf("inputs.timeout: got %v, want %q", vals.Inputs["timeout"], "30s")
	}
	if len(vals.Env) != 0 {
		t.Errorf("env: got %d entries, want 0", len(vals.Env))
	}
}

func TestLoadValues_EnvOnly(t *testing.T) {
	path := writeValuesFile(t, `
env:
  DATABASE_URL: postgres://localhost/mydb
`)

	vals, err := LoadValues(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(vals.Env) != 1 {
		t.Errorf("env: got %d entries, want 1", len(vals.Env))
	}
	if vals.Env["DATABASE_URL"] != "postgres://localhost/mydb" {
		t.Errorf("env.DATABASE_URL: got %q, want %q", vals.Env["DATABASE_URL"], "postgres://localhost/mydb")
	}
	if len(vals.Inputs) != 0 {
		t.Errorf("inputs: got %d entries, want 0", len(vals.Inputs))
	}
}

func TestLoadValues_EnvKeysNormalizedToUppercase(t *testing.T) {
	path := writeValuesFile(t, `
env:
  api_key: lowercase_key
  Api_Secret: mixed_case
  ALREADY_UPPER: already_upper
`)

	vals, err := LoadValues(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if vals.Env["API_KEY"] != "lowercase_key" {
		t.Errorf("env.API_KEY: got %q, want %q", vals.Env["API_KEY"], "lowercase_key")
	}
	if vals.Env["API_SECRET"] != "mixed_case" {
		t.Errorf("env.API_SECRET: got %q, want %q", vals.Env["API_SECRET"], "mixed_case")
	}
	if vals.Env["ALREADY_UPPER"] != "already_upper" {
		t.Errorf("env.ALREADY_UPPER: got %q, want %q", vals.Env["ALREADY_UPPER"], "already_upper")
	}
	// Original lowercase keys should not be present.
	if _, ok := vals.Env["api_key"]; ok {
		t.Error("lowercase key 'api_key' should have been normalized away")
	}
	if _, ok := vals.Env["Api_Secret"]; ok {
		t.Error("mixed-case key 'Api_Secret' should have been normalized away")
	}
}

func TestLoadValues_EmptyFile(t *testing.T) {
	path := writeValuesFile(t, "")

	vals, err := LoadValues(path)
	if err != nil {
		t.Fatalf("unexpected error for empty file: %v", err)
	}

	if len(vals.Inputs) != 0 {
		t.Errorf("inputs: got %d, want 0", len(vals.Inputs))
	}
	if len(vals.Env) != 0 {
		t.Errorf("env: got %d, want 0", len(vals.Env))
	}
}

// --- MergeInputs tests ---

func TestMergeInputs_FullPrecedence(t *testing.T) {
	workflowInputs := map[string]Input{
		"url":     {Type: "string", Default: "https://default.example.com"},
		"retries": {Type: "number", Default: 3},
		"timeout": {Type: "string", Default: "30s"},
	}
	valuesInputs := map[string]any{
		"url":     "https://staging.example.com",
		"retries": 5,
	}
	inlineInputs := map[string]any{
		"url": "https://prod.example.com",
	}

	result := MergeInputs(workflowInputs, nil, valuesInputs, inlineInputs)

	// Inline wins over values and defaults for url.
	if result["url"] != "https://prod.example.com" {
		t.Errorf("url: got %v, want %q", result["url"], "https://prod.example.com")
	}
	// Values wins over defaults for retries.
	if result["retries"] != 5 {
		t.Errorf("retries: got %v, want 5", result["retries"])
	}
	// Default used for timeout (not overridden).
	if result["timeout"] != "30s" {
		t.Errorf("timeout: got %v, want %q", result["timeout"], "30s")
	}
}

func TestMergeInputs_NilLayers(t *testing.T) {
	result := MergeInputs(nil, nil, nil, nil)

	if result == nil {
		t.Fatal("result should not be nil")
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d entries", len(result))
	}
}

func TestMergeInputs_InlineOverridesWithNoDefault(t *testing.T) {
	workflowInputs := map[string]Input{
		"name": {Type: "string"}, // no default
	}
	inlineInputs := map[string]any{
		"name": "alice",
	}

	result := MergeInputs(workflowInputs, nil, nil, inlineInputs)

	if result["name"] != "alice" {
		t.Errorf("name: got %v, want %q", result["name"], "alice")
	}
}

func TestMergeInputs_ValuesWithNoDefault(t *testing.T) {
	workflowInputs := map[string]Input{
		"region": {Type: "string"}, // no default
	}
	valuesInputs := map[string]any{
		"region": "us-east-1",
	}

	result := MergeInputs(workflowInputs, nil, valuesInputs, nil)

	if result["region"] != "us-east-1" {
		t.Errorf("region: got %v, want %q", result["region"], "us-east-1")
	}
}

func TestMergeInputs_DefaultsOnlyWhenNoOverrides(t *testing.T) {
	workflowInputs := map[string]Input{
		"verbose": {Type: "boolean", Default: false},
		"limit":   {Type: "number", Default: 100},
	}

	result := MergeInputs(workflowInputs, nil, nil, nil)

	if result["verbose"] != false {
		t.Errorf("verbose: got %v, want false", result["verbose"])
	}
	if result["limit"] != 100 {
		t.Errorf("limit: got %v, want 100", result["limit"])
	}
}

func TestMergeInputs_InputsWithNilDefaultNotIncluded(t *testing.T) {
	workflowInputs := map[string]Input{
		"required_input": {Type: "string"}, // no default, nil Default
	}

	result := MergeInputs(workflowInputs, nil, nil, nil)

	if _, ok := result["required_input"]; ok {
		t.Error("input with nil Default should not appear in result")
	}
}

func TestMergeInputs_FourLayerPrecedence(t *testing.T) {
	workflowInputs := map[string]Input{
		"url":     {Type: "string", Default: "https://default.example.com"},
		"retries": {Type: "number", Default: 3},
		"timeout": {Type: "string", Default: "30s"},
		"region":  {Type: "string", Default: "us-west-2"},
	}
	envInputs := map[string]any{
		"url":     "https://env.example.com",
		"retries": 5,
		"region":  "eu-west-1",
	}
	valuesInputs := map[string]any{
		"url":     "https://values.example.com",
		"retries": 10,
	}
	inlineInputs := map[string]any{
		"url": "https://inline.example.com",
	}

	result := MergeInputs(workflowInputs, envInputs, valuesInputs, inlineInputs)

	// Inline wins for url.
	if result["url"] != "https://inline.example.com" {
		t.Errorf("url = %v, want %q (inline wins)", result["url"], "https://inline.example.com")
	}
	// Values wins over env for retries.
	if result["retries"] != 10 {
		t.Errorf("retries = %v, want 10 (values wins over env)", result["retries"])
	}
	// Env wins over default for region.
	if result["region"] != "eu-west-1" {
		t.Errorf("region = %v, want %q (env wins over default)", result["region"], "eu-west-1")
	}
	// Default used for timeout.
	if result["timeout"] != "30s" {
		t.Errorf("timeout = %v, want %q (default)", result["timeout"], "30s")
	}
}
