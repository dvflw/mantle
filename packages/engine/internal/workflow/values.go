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

// MergeInputs combines input values with the following precedence (highest wins):
//
//	inline --input flags > values file inputs > named environment inputs > workflow definition defaults
//
// All layers are optional (nil-safe).
func MergeInputs(workflowInputs map[string]Input, envInputs map[string]any, valuesInputs map[string]any, inlineInputs map[string]any) map[string]any {
	result := make(map[string]any)

	// Layer 1: workflow definition defaults (lowest precedence).
	for name, input := range workflowInputs {
		if input.Default != nil {
			result[name] = input.Default
		}
	}

	// Layer 2: named environment inputs.
	for k, v := range envInputs {
		result[k] = v
	}

	// Layer 3: values file inputs.
	for k, v := range valuesInputs {
		result[k] = v
	}

	// Layer 4: inline --input flags (highest precedence).
	for k, v := range inlineInputs {
		result[k] = v
	}

	return result
}
