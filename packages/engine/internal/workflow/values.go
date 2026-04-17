package workflow

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Values holds environment-specific overrides loaded from a values file.
type Values struct {
	Inputs map[string]any    `yaml:"inputs"`
	Env    map[string]string `yaml:"env"`
}

// LoadValues reads and validates a values file from disk.
func LoadValues(path string) (*Values, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading values file %s: %w", path, err)
	}

	return LoadValuesBytes(data)
}

// LoadValuesBytes parses and validates values from raw YAML bytes.
func LoadValuesBytes(data []byte) (*Values, error) {
	// First decode to a generic map to detect unknown keys.
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing values file: %w", err)
	}

	allowed := map[string]bool{"inputs": true, "env": true}
	for key := range raw {
		if !allowed[key] {
			return nil, fmt.Errorf("unknown key %q in values file (allowed: inputs, env)", key)
		}
	}

	var vals Values
	if err := yaml.Unmarshal(data, &vals); err != nil {
		return nil, fmt.Errorf("parsing values file: %w", err)
	}

	// Normalize env keys to uppercase (matches config.go behavior).
	// Detect collisions where different-cased keys map to the same uppercase key.
	if len(vals.Env) > 0 {
		normalized := make(map[string]string, len(vals.Env))
		sources := make(map[string]string, len(vals.Env)) // uppercase key -> original key
		for k, v := range vals.Env {
			upper := strings.ToUpper(k)
			if orig, exists := sources[upper]; exists && orig != k {
				return nil, fmt.Errorf("env key collision: %q and %q both normalize to %q", orig, k, upper)
			}
			sources[upper] = k
			normalized[upper] = v
		}
		vals.Env = normalized
	}

	return &vals, nil
}

// Source identifies which override layer supplied a resolved value.
type Source string

const (
	SourceDefault Source = "default"
	SourceConfig  Source = "config"
	SourceEnv     Source = "env"
	SourceValues  Source = "values"
	SourceInline  Source = "inline"
)

// Resolved pairs a final merged value with the layer it originated from.
type Resolved struct {
	Value  any
	Source Source
}

// MergeInputs combines input values with the following precedence (highest wins):
//
//	inline --input flags > values file inputs > named environment inputs > workflow definition defaults
//
// All layers are optional (nil-safe).
func MergeInputs(workflowInputs map[string]Input, envInputs map[string]any, valuesInputs map[string]any, inlineInputs map[string]any) map[string]any {
	merged := ResolveInputs(workflowInputs, envInputs, valuesInputs, inlineInputs)
	result := make(map[string]any, len(merged))
	for k, v := range merged {
		result[k] = v.Value
	}
	return result
}

// ResolveInputs is like MergeInputs but preserves which layer each final value
// came from. Callers that need to display "where did this value come from?"
// (e.g., `mantle plan --env`) should use this.
func ResolveInputs(workflowInputs map[string]Input, envInputs map[string]any, valuesInputs map[string]any, inlineInputs map[string]any) map[string]Resolved {
	result := make(map[string]Resolved)

	for name, input := range workflowInputs {
		if input.Default != nil {
			result[name] = Resolved{Value: input.Default, Source: SourceDefault}
		}
	}
	for k, v := range envInputs {
		result[k] = Resolved{Value: v, Source: SourceEnv}
	}
	for k, v := range valuesInputs {
		result[k] = Resolved{Value: v, Source: SourceValues}
	}
	for k, v := range inlineInputs {
		result[k] = Resolved{Value: v, Source: SourceInline}
	}

	return result
}

// ResolveEnvVars merges env-var layers (config < named env < values file) and
// records the source of each final value. MANTLE_ENV_* OS variables are not
// resolved here because plan runs before execution and OS env can change.
func ResolveEnvVars(configEnv map[string]string, envEnv map[string]string, valuesEnv map[string]string) map[string]Resolved {
	result := make(map[string]Resolved)
	for k, v := range configEnv {
		result[k] = Resolved{Value: v, Source: SourceConfig}
	}
	for k, v := range envEnv {
		result[k] = Resolved{Value: v, Source: SourceEnv}
	}
	for k, v := range valuesEnv {
		result[k] = Resolved{Value: v, Source: SourceValues}
	}
	return result
}
