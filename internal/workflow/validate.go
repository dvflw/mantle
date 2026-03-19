package workflow

import (
	"fmt"
	"regexp"
	"sort"
	"time"

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
