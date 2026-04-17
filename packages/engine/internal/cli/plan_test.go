package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dvflw/mantle/internal/workflow"
)

func TestWriteResolvedOverrides_NoOverrides(t *testing.T) {
	var buf bytes.Buffer
	writeResolvedOverrides(&buf, nil, nil, nil, nil, nil, nil)
	if buf.Len() != 0 {
		t.Errorf("expected no output when no overrides supplied, got: %q", buf.String())
	}
}

func TestWriteResolvedOverrides_ShowsSourcePerValue(t *testing.T) {
	wfInputs := map[string]workflow.Input{
		"url":     {Type: "string", Default: "https://default.example.com"},
		"retries": {Type: "number", Default: 3},
	}
	envInputs := map[string]any{
		"url": "https://env.example.com",
	}
	valuesInputs := map[string]any{
		"retries": 10,
	}
	configEnv := map[string]string{"LOG_LEVEL": "info"}
	envEnvVars := map[string]string{"REGION": "us-east-1"}
	valuesEnv := map[string]string{"LOG_LEVEL": "debug"}

	var buf bytes.Buffer
	writeResolvedOverrides(&buf, wfInputs, envInputs, valuesInputs, configEnv, envEnvVars, valuesEnv)
	out := buf.String()

	checks := []string{
		"Resolved configuration:",
		"url = https://env.example.com  (from: env)",
		"retries = 10  (from: values)",
		"LOG_LEVEL = \"debug\"  (from: values)",
		"REGION = \"us-east-1\"  (from: env)",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q\nfull output:\n%s", want, out)
		}
	}
}

func TestWriteResolvedOverrides_ValuesOnly(t *testing.T) {
	valuesInputs := map[string]any{"name": "staging"}

	var buf bytes.Buffer
	writeResolvedOverrides(&buf, nil, nil, valuesInputs, nil, nil, nil)
	out := buf.String()

	if !strings.Contains(out, "name = staging  (from: values)") {
		t.Errorf("expected values source attribution, got: %s", out)
	}
}

func TestWriteResolvedOverrides_EnvVarsOnlyOmitsInputsBlock(t *testing.T) {
	wfInputs := map[string]workflow.Input{
		"url": {Type: "string", Default: "https://default.example.com"},
	}
	envEnvVars := map[string]string{"REGION": "eu-west-1"}

	var buf bytes.Buffer
	writeResolvedOverrides(&buf, wfInputs, nil, nil, nil, envEnvVars, nil)
	out := buf.String()

	if strings.Contains(out, "Inputs:") {
		t.Errorf("did not expect Inputs block when only env vars overridden, got:\n%s", out)
	}
	if !strings.Contains(out, "REGION = \"eu-west-1\"  (from: env)") {
		t.Errorf("expected env vars block, got:\n%s", out)
	}
}

func TestWriteResolvedOverrides_ConfigEnvUsesTypedSource(t *testing.T) {
	configEnv := map[string]string{"LOG_LEVEL": "info"}
	// At least one override layer so the block prints.
	envEnvVars := map[string]string{"REGION": "us-east-1"}

	var buf bytes.Buffer
	writeResolvedOverrides(&buf, nil, nil, nil, configEnv, envEnvVars, nil)
	out := buf.String()

	if !strings.Contains(out, "LOG_LEVEL = \"info\"  (from: config)") {
		t.Errorf("expected config source attribution for LOG_LEVEL, got:\n%s", out)
	}
}
