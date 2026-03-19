package workflow

import (
	"fmt"
	"regexp"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	namePattern  = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	inputPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

	validInputTypes   = map[string]bool{"string": true, "number": true, "boolean": true}
	validBackoffTypes = map[string]bool{"fixed": true, "exponential": true}
)

// Validate performs structural validation on a parsed workflow and returns
// any validation errors found. It checks naming conventions, required fields,
// input types, retry policies, and timeout durations.
func Validate(result *ParseResult) []ValidationError {
	var errs []ValidationError
	w := result.Workflow
	root := result.Root

	// Validate workflow name.
	if w.Name == "" {
		line, col := findFieldPosition(root, "name")
		errs = append(errs, ValidationError{
			Line: line, Column: col, Field: "name",
			Message: "name is required",
		})
	} else if !namePattern.MatchString(w.Name) {
		line, col := findFieldPosition(root, "name")
		errs = append(errs, ValidationError{
			Line: line, Column: col, Field: "name",
			Message: "name must match ^[a-z][a-z0-9-]*$",
		})
	}

	// Validate steps exist and are non-empty.
	if len(w.Steps) == 0 {
		line, col := findFieldPosition(root, "steps")
		errs = append(errs, ValidationError{
			Line: line, Column: col, Field: "steps",
			Message: "at least one step is required",
		})
	}

	// Validate inputs.
	// Sort input names for deterministic error ordering.
	inputNames := make([]string, 0, len(w.Inputs))
	for name := range w.Inputs {
		inputNames = append(inputNames, name)
	}
	sort.Strings(inputNames)

	for _, name := range inputNames {
		inp := w.Inputs[name]
		if !inputPattern.MatchString(name) {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("inputs.%s", name),
				Message: "input name must match ^[a-z][a-z0-9_]*$",
			})
		}
		if inp.Type != "" && !validInputTypes[inp.Type] {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("inputs.%s.type", name),
				Message: fmt.Sprintf("type must be one of: string, number, boolean (got %q)", inp.Type),
			})
		}
	}

	// Validate individual steps.
	seen := make(map[string]bool)
	for i, step := range w.Steps {
		prefix := fmt.Sprintf("steps[%d]", i)

		if step.Name == "" {
			errs = append(errs, ValidationError{
				Field:   prefix + ".name",
				Message: "step name is required",
			})
		} else {
			if !namePattern.MatchString(step.Name) {
				errs = append(errs, ValidationError{
					Field:   prefix + ".name",
					Message: "step name must match ^[a-z][a-z0-9-]*$",
				})
			}
			if seen[step.Name] {
				errs = append(errs, ValidationError{
					Field:   prefix + ".name",
					Message: fmt.Sprintf("duplicate step name %q", step.Name),
				})
			}
			seen[step.Name] = true
		}

		if step.Action == "" {
			errs = append(errs, ValidationError{
				Field:   prefix + ".action",
				Message: "step action is required",
			})
		}

		// Validate retry policy.
		if step.Retry != nil {
			if step.Retry.MaxAttempts <= 0 {
				errs = append(errs, ValidationError{
					Field:   prefix + ".retry.max_attempts",
					Message: "max_attempts must be greater than 0",
				})
			}
			if step.Retry.Backoff != "" && !validBackoffTypes[step.Retry.Backoff] {
				errs = append(errs, ValidationError{
					Field:   prefix + ".retry.backoff",
					Message: fmt.Sprintf("backoff must be one of: fixed, exponential (got %q)", step.Retry.Backoff),
				})
			}
		}

		// Validate timeout.
		if step.Timeout != "" {
			d, err := time.ParseDuration(step.Timeout)
			if err != nil {
				errs = append(errs, ValidationError{
					Field:   prefix + ".timeout",
					Message: fmt.Sprintf("invalid duration: %v", err),
				})
			} else if d <= 0 {
				errs = append(errs, ValidationError{
					Field:   prefix + ".timeout",
					Message: "timeout must be a positive duration",
				})
			}
		}

		// Validate tools for ai/completion steps.
		if step.Action == "ai/completion" && step.Params != nil {
			tools, err := ParseTools(step.Params)
			if err != nil {
				errs = append(errs, ValidationError{
					Field:   prefix + ".params.tools",
					Message: fmt.Sprintf("invalid tools: %v", err),
				})
			} else if len(tools) > 0 {
				toolNames := make(map[string]bool)
				for j, tool := range tools {
					toolPrefix := fmt.Sprintf("%s.params.tools[%d]", prefix, j)

					if tool.Name == "" {
						errs = append(errs, ValidationError{
							Field:   toolPrefix + ".name",
							Message: "tool name is required",
						})
					} else {
						if toolNames[tool.Name] {
							errs = append(errs, ValidationError{
								Field:   toolPrefix + ".name",
								Message: fmt.Sprintf("duplicate tool name %q", tool.Name),
							})
						}
						toolNames[tool.Name] = true
					}

					if tool.Description == "" {
						errs = append(errs, ValidationError{
							Field:   toolPrefix + ".description",
							Message: "tool description is required for LLM function calling",
						})
					}

					if tool.InputSchema == nil {
						errs = append(errs, ValidationError{
							Field:   toolPrefix + ".input_schema",
							Message: "tool input_schema is required",
						})
					}

					if tool.Action == "" {
						errs = append(errs, ValidationError{
							Field:   toolPrefix + ".action",
							Message: "tool action is required",
						})
					}
				}
			}

			// Validate max_tool_rounds.
			if v, ok := step.Params["max_tool_rounds"]; ok {
				if rounds, ok := toInt(v); ok {
					if rounds > 50 {
						errs = append(errs, ValidationError{
							Field:   prefix + ".params.max_tool_rounds",
							Message: "max_tool_rounds must not exceed 50",
						})
					}
				}
			}

			// Validate max_tool_calls_per_round.
			if v, ok := step.Params["max_tool_calls_per_round"]; ok {
				if calls, ok := toInt(v); ok {
					if calls > 25 {
						errs = append(errs, ValidationError{
							Field:   prefix + ".params.max_tool_calls_per_round",
							Message: "max_tool_calls_per_round must not exceed 25",
						})
					}
				}
			}
		}
	}

	return errs
}

// toInt attempts to convert a value from YAML parsing to an integer.
// YAML numbers may be decoded as int or float64 depending on format.
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		return int(n), true
	}
	return 0, false
}

// findFieldPosition searches the root mapping node for a top-level key and
// returns its line and column. Falls back to (0, 0) if not found.
func findFieldPosition(root *yaml.Node, field string) (int, int) {
	if root.Kind != yaml.MappingNode {
		return 0, 0
	}
	for i := 0; i < len(root.Content)-1; i += 2 {
		if root.Content[i].Value == field {
			return root.Content[i].Line, root.Content[i].Column
		}
	}
	return 0, 0
}
