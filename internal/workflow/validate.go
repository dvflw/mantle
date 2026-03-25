package workflow

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	mantleCEL "github.com/dvflw/mantle/internal/cel"
	"gopkg.in/yaml.v3"
)

var (
	namePattern    = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	inputPattern   = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	stepRefPattern = regexp.MustCompile(`steps\.(\w+)`)

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

	// Validate token_budget is non-negative.
	if w.TokenBudget < 0 {
		line, col := findFieldPosition(root, "token_budget")
		errs = append(errs, ValidationError{
			Line: line, Column: col, Field: "token_budget",
			Message: fmt.Sprintf("token_budget must be >= 0, got %d", w.TokenBudget),
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

	// Validate workflow-level timeout.
	if w.Timeout != "" {
		d, err := time.ParseDuration(w.Timeout)
		if err != nil {
			errs = append(errs, ValidationError{
				Field:   "timeout",
				Message: fmt.Sprintf("invalid duration: %v", err),
			})
		} else if d <= 0 {
			errs = append(errs, ValidationError{
				Field:   "timeout",
				Message: "timeout must be a positive duration",
			})
		}
	}

	// Validate triggers.
	validTriggerTypes := map[string]bool{"cron": true, "webhook": true, "email": true}
	validEmailFilters := map[string]bool{"unseen": true, "all": true, "flagged": true, "recent": true}
	for i, trig := range w.Triggers {
		trigPrefix := fmt.Sprintf("triggers[%d]", i)
		if trig.Type == "" {
			errs = append(errs, ValidationError{
				Field:   trigPrefix + ".type",
				Message: "trigger type is required",
			})
			continue
		}
		if !validTriggerTypes[trig.Type] {
			errs = append(errs, ValidationError{
				Field:   trigPrefix + ".type",
				Message: fmt.Sprintf("trigger type must be one of: cron, webhook, email (got %q)", trig.Type),
			})
			continue
		}
		switch trig.Type {
		case "email":
			if trig.Mailbox == "" {
				errs = append(errs, ValidationError{
					Field:   trigPrefix + ".mailbox",
					Message: "mailbox is required for email triggers",
				})
			}
			if trig.Filter != "" && !validEmailFilters[trig.Filter] {
				errs = append(errs, ValidationError{
					Field:   trigPrefix + ".filter",
					Message: fmt.Sprintf("filter must be one of: unseen, all, flagged, recent (got %q)", trig.Filter),
				})
			}
			if trig.PollInterval != "" {
				d, err := time.ParseDuration(trig.PollInterval)
				if err != nil {
					errs = append(errs, ValidationError{
						Field:   trigPrefix + ".poll_interval",
						Message: fmt.Sprintf("invalid duration: %v", err),
					})
				} else if d <= 0 {
					errs = append(errs, ValidationError{
						Field:   trigPrefix + ".poll_interval",
						Message: "poll_interval must be a positive duration",
					})
				}
			}
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

	// Validate artifact declarations.
	artifactNames := make(map[string]string) // name -> step that declared it
	for i, step := range w.Steps {
		prefix := fmt.Sprintf("steps[%d]", i)
		for j, art := range step.Artifacts {
			artPrefix := fmt.Sprintf("%s.artifacts[%d]", prefix, j)
			if art.Name == "" {
				errs = append(errs, ValidationError{
					Field:   artPrefix + ".name",
					Message: "artifact name is required",
				})
			}
			if art.Path == "" {
				errs = append(errs, ValidationError{
					Field:   artPrefix + ".path",
					Message: "artifact path is required",
				})
			}
			if art.Name != "" {
				if prevStep, exists := artifactNames[art.Name]; exists {
					errs = append(errs, ValidationError{
						Field:   artPrefix + ".name",
						Message: fmt.Sprintf("duplicate artifact name %q (also declared in step %q)", art.Name, prevStep),
					})
				} else {
					artifactNames[art.Name] = step.Name
				}
			}
		}
	}

	// Validate dependency references (explicit + implicit from CEL expressions).
	for i, step := range w.Steps {
		prefix := fmt.Sprintf("steps[%d]", i)
		allDeps := mergeUnique(step.DependsOn, ExtractImplicitDeps(step))
		for _, dep := range allDeps {
			if !seen[dep] {
				errs = append(errs, ValidationError{
					Field:   prefix + ".depends_on",
					Message: fmt.Sprintf("references undefined step %q", dep),
				})
			}
		}
	}

	// Validate CEL expression syntax in step params and if conditions.
	celEval, celErr := mantleCEL.NewEvaluator()
	if celErr == nil {
		for i, step := range w.Steps {
			prefix := fmt.Sprintf("steps[%d]", i)

			// Check if condition.
			if step.If != "" {
				if err := celEval.CompileCheck(step.If); err != nil {
					errs = append(errs, ValidationError{
						Field:   prefix + ".if",
						Message: fmt.Sprintf("invalid CEL expression: %v", err),
					})
				}
			}

			// Check params recursively.
			errs = append(errs, validateCELInParams(celEval, step.Params, prefix+".params")...)
		}
	}

	return errs
}

// ExtractImplicitDeps extracts step names referenced in CEL expressions within
// a step's If condition and Params values. It uses regex matching to find
// static references of the form steps.<name>. Results are deduplicated.
func ExtractImplicitDeps(step Step) []string {
	seen := make(map[string]bool)
	var refs []string

	// Scan the If condition.
	if step.If != "" {
		for _, match := range stepRefPattern.FindAllStringSubmatch(step.If, -1) {
			name := match[1]
			if !seen[name] {
				seen[name] = true
				refs = append(refs, name)
			}
		}
	}

	// Scan params recursively.
	scanParamsForRefs(step.Params, seen, &refs)

	return refs
}

// scanParamsForRefs walks a params map recursively, extracting step references
// from string values using the stepRefPattern regex.
func scanParamsForRefs(params map[string]any, seen map[string]bool, refs *[]string) {
	for _, v := range params {
		switch val := v.(type) {
		case string:
			for _, match := range stepRefPattern.FindAllStringSubmatch(val, -1) {
				name := match[1]
				if !seen[name] {
					seen[name] = true
					*refs = append(*refs, name)
				}
			}
		case map[string]any:
			scanParamsForRefs(val, seen, refs)
		}
	}
}

// mergeUnique combines two string slices, returning a new slice with no duplicates.
// Order is preserved: elements from a appear first, then new elements from b.
func mergeUnique(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	var result []string
	for _, s := range a {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	for _, s := range b {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
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

// validateCELInParams recursively walks a params map and validates any CEL
// expressions found inside {{ }} delimiters.
func validateCELInParams(eval *mantleCEL.Evaluator, params map[string]any, prefix string) []ValidationError {
	var errs []ValidationError
	for k, v := range params {
		field := prefix + "." + k
		errs = append(errs, validateCELInValue(eval, v, field)...)
	}
	return errs
}

// validateCELInValue recursively checks a value for embedded CEL expressions.
func validateCELInValue(eval *mantleCEL.Evaluator, v any, field string) []ValidationError {
	var errs []ValidationError
	switch val := v.(type) {
	case string:
		for _, expr := range extractCELExpressions(val) {
			if err := eval.CompileCheck(expr); err != nil {
				errs = append(errs, ValidationError{
					Field:   field,
					Message: fmt.Sprintf("invalid CEL expression %q: %v", expr, err),
				})
			}
		}
	case map[string]any:
		for k, child := range val {
			errs = append(errs, validateCELInValue(eval, child, field+"."+k)...)
		}
	case []any:
		for i, item := range val {
			errs = append(errs, validateCELInValue(eval, item, fmt.Sprintf("%s[%d]", field, i))...)
		}
	}
	return errs
}

// extractCELExpressions extracts CEL expressions from {{ }} delimiters in a string.
func extractCELExpressions(s string) []string {
	var exprs []string
	remaining := s
	for {
		start := strings.Index(remaining, "{{")
		if start == -1 {
			break
		}
		end := strings.Index(remaining[start:], "}}")
		if end == -1 {
			break
		}
		end += start
		expr := strings.TrimSpace(remaining[start+2 : end])
		if expr != "" {
			exprs = append(exprs, expr)
		}
		remaining = remaining[end+2:]
	}
	return exprs
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
