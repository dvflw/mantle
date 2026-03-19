package cel

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

// Context holds the runtime data available to CEL expressions.
type Context struct {
	Steps  map[string]map[string]any // steps.<name>.output → step outputs
	Inputs map[string]any            // inputs.<name> → workflow inputs
}

// Evaluator evaluates CEL expressions against a runtime context.
type Evaluator struct {
	env      *cel.Env
	envCache map[string]string // cached, filtered environment variables
}

// NewEvaluator creates a CEL evaluator with the standard Mantle expression environment.
func NewEvaluator() (*Evaluator, error) {
	env, err := cel.NewEnv(
		cel.Variable("steps", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("inputs", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("env", cel.MapType(cel.StringType, cel.StringType)),
	)
	if err != nil {
		return nil, fmt.Errorf("creating CEL environment: %w", err)
	}
	return &Evaluator{env: env, envCache: envVars()}, nil
}

// Eval evaluates a CEL expression and returns the result as a Go value.
func (e *Evaluator) Eval(expression string, ctx *Context) (any, error) {
	ast, issues := e.env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("compiling expression %q: %w", expression, issues.Err())
	}

	prog, err := e.env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("creating program for %q: %w", expression, err)
	}

	vars := map[string]any{
		"steps":  ctx.Steps,
		"inputs": ctx.Inputs,
		"env":    e.envCache,
	}

	out, _, err := prog.Eval(vars)
	if err != nil {
		return nil, fmt.Errorf("evaluating %q: %w", expression, err)
	}

	return refToNative(out), nil
}

// EvalBool evaluates a CEL expression that should return a boolean.
func (e *Evaluator) EvalBool(expression string, ctx *Context) (bool, error) {
	result, err := e.Eval(expression, ctx)
	if err != nil {
		return false, err
	}
	b, ok := result.(bool)
	if !ok {
		return false, fmt.Errorf("expression %q returned %T, expected bool", expression, result)
	}
	return b, nil
}

// ResolveString resolves CEL expressions embedded in a string value.
// Expressions are delimited by {{ and }}. If the entire string is a single
// expression, the raw result is returned (preserving type). Otherwise,
// expressions are interpolated into the string.
func (e *Evaluator) ResolveString(value string, ctx *Context) (any, error) {
	trimmed := strings.TrimSpace(value)

	// If the entire value is a single {{ expr }}, evaluate and return the raw result.
	if strings.HasPrefix(trimmed, "{{") && strings.HasSuffix(trimmed, "}}") {
		inner := strings.Count(trimmed, "{{")
		if inner == 1 {
			expr := strings.TrimSpace(trimmed[2 : len(trimmed)-2])
			return e.Eval(expr, ctx)
		}
	}

	// Otherwise, interpolate all {{ expr }} occurrences into the string.
	result := value
	for {
		start := strings.Index(result, "{{")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], "}}")
		if end == -1 {
			break
		}
		end += start + 2

		expr := strings.TrimSpace(result[start+2 : end-2])
		val, err := e.Eval(expr, ctx)
		if err != nil {
			return nil, err
		}
		result = result[:start] + fmt.Sprintf("%v", val) + result[end:]
	}

	return result, nil
}

// ResolveParams recursively resolves CEL expressions in a params map.
func (e *Evaluator) ResolveParams(params map[string]any, ctx *Context) (map[string]any, error) {
	resolved := make(map[string]any, len(params))
	for k, v := range params {
		r, err := e.resolveValue(v, ctx)
		if err != nil {
			return nil, fmt.Errorf("resolving param %q: %w", k, err)
		}
		resolved[k] = r
	}
	return resolved, nil
}

func (e *Evaluator) resolveValue(v any, ctx *Context) (any, error) {
	switch val := v.(type) {
	case string:
		return e.ResolveString(val, ctx)
	case map[string]any:
		return e.ResolveParams(val, ctx)
	case []any:
		result := make([]any, len(val))
		for i, item := range val {
			r, err := e.resolveValue(item, ctx)
			if err != nil {
				return nil, err
			}
			result[i] = r
		}
		return result, nil
	default:
		return v, nil
	}
}

// envVars returns only environment variables with the MANTLE_ENV_ prefix,
// stripping the prefix for cleaner access (e.g., MANTLE_ENV_FOO -> env.FOO).
// This prevents CEL expressions from reading sensitive variables like
// MANTLE_ENCRYPTION_KEY, MANTLE_DATABASE_URL, or AWS_SECRET_ACCESS_KEY.
func envVars() map[string]string {
	env := make(map[string]string)
	const prefix = "MANTLE_ENV_"
	for _, e := range os.Environ() {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 && strings.HasPrefix(parts[0], prefix) {
			key := strings.TrimPrefix(parts[0], prefix)
			env[key] = parts[1]
		}
	}
	return env
}

// refToNative converts a CEL ref.Val to a native Go value.
func refToNative(v ref.Val) any {
	switch v.Type() {
	case types.BoolType:
		return v.Value()
	case types.IntType:
		return v.Value()
	case types.DoubleType:
		return v.Value()
	case types.StringType:
		return v.Value()
	case types.MapType:
		nv, err := v.ConvertToNative(mapType)
		if err != nil {
			return v.Value()
		}
		return nv
	case types.ListType:
		nv, err := v.ConvertToNative(sliceType)
		if err != nil {
			return v.Value()
		}
		return nv
	default:
		return v.Value()
	}
}

var (
	mapType   = reflect.TypeOf(map[string]any{})
	sliceType = reflect.TypeOf([]any{})
)
