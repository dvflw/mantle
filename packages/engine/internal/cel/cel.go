package cel

import (
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"strings"
	"sync"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

// Context holds the runtime data available to CEL expressions.
type Context struct {
	Steps     map[string]map[string]any // steps.<name>.output → step outputs
	Inputs    map[string]any            // inputs.<name> → workflow inputs
	Trigger   map[string]any            // trigger.payload → webhook trigger data
	Artifacts map[string]map[string]any // artifacts.<name> → {name, url, size}
}

const maxProgramCacheSize = 10000

// Evaluator evaluates CEL expressions against a runtime context.
type Evaluator struct {
	env          *cel.Env
	configEnv    map[string]string      // config-sourced env values from mantle.yaml
	envCache     map[string]string      // cached, filtered environment variables
	programMu    sync.Mutex
	programCache map[string]cel.Program // expression string -> compiled cel.Program
}

// NewEvaluator creates a CEL evaluator with the standard Mantle expression environment.
func NewEvaluator() (*Evaluator, error) {
	opts := []cel.EnvOption{
		cel.Variable("steps", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("inputs", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("env", cel.MapType(cel.StringType, cel.StringType)),
		cel.Variable("trigger", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("artifacts", cel.MapType(cel.StringType, cel.DynType)),
	}
	opts = append(opts, customFunctions()...)

	env, err := cel.NewEnv(opts...)
	if err != nil {
		return nil, fmt.Errorf("creating CEL environment: %w", err)
	}
	return &Evaluator{env: env, envCache: mergeEnvVars(nil), programCache: make(map[string]cel.Program)}, nil
}

// Eval evaluates a CEL expression and returns the result as a Go value.
// Compiled programs are cached by expression string to avoid redundant compilation.
func (e *Evaluator) Eval(expression string, ctx *Context) (any, error) {
	prog, err := e.getOrCompile(expression)
	if err != nil {
		return nil, err
	}

	trigger := ctx.Trigger
	if trigger == nil {
		trigger = map[string]any{}
	}

	artifacts := ctx.Artifacts
	if artifacts == nil {
		artifacts = map[string]map[string]any{}
	}

	vars := map[string]any{
		"steps":     ctx.Steps,
		"inputs":    ctx.Inputs,
		"env":       e.envCache,
		"trigger":   trigger,
		"artifacts": artifacts,
	}

	out, _, err := prog.Eval(vars)
	if err != nil {
		return nil, fmt.Errorf("evaluating %q: %w", expression, err)
	}

	return refToNative(out), nil
}

// getOrCompile returns a cached compiled program or compiles and caches a new one.
// The cache is bounded to maxProgramCacheSize entries; when full it is cleared
// (simple eviction — sufficient for the expected expression cardinality).
func (e *Evaluator) getOrCompile(expression string) (cel.Program, error) {
	e.programMu.Lock()
	if cached, ok := e.programCache[expression]; ok {
		e.programMu.Unlock()
		return cached, nil
	}
	e.programMu.Unlock()

	ast, issues := e.env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("compiling expression %q: %w", expression, issues.Err())
	}

	prog, err := e.env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("creating program for %q: %w", expression, err)
	}

	e.programMu.Lock()
	if len(e.programCache) >= maxProgramCacheSize {
		e.programCache = make(map[string]cel.Program)
	}
	e.programCache[expression] = prog
	e.programMu.Unlock()
	return prog, nil
}

// CompileCheck compiles a CEL expression and returns any syntax/type errors
// without evaluating it. This is used for offline validation.
func (e *Evaluator) CompileCheck(expression string) error {
	_, err := e.getOrCompile(expression)
	return err
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

// SetConfigEnv sets the config-sourced env values and rebuilds the env cache.
// This merges config values with MANTLE_ENV_* environment variables, where
// OS env vars take precedence over config values.
func (e *Evaluator) SetConfigEnv(configEnv map[string]string) {
	e.configEnv = configEnv
	e.envCache = mergeEnvVars(configEnv)
}

// mergeEnvVars builds the env map available to CEL expressions.
// It starts with config-sourced values (from the env: section in mantle.yaml),
// then overlays MANTLE_ENV_* environment variables (stripping the prefix).
// When a key exists in both, the OS env var wins and an info log is emitted.
// This prevents CEL expressions from reading sensitive variables like
// MANTLE_ENCRYPTION_KEY, MANTLE_DATABASE_URL, or AWS_SECRET_ACCESS_KEY.
func mergeEnvVars(configEnv map[string]string) map[string]string {
	env := make(map[string]string)

	// Start with config values.
	for k, v := range configEnv {
		env[k] = v
	}

	// Overlay MANTLE_ENV_* from OS environment (env var wins).
	const prefix = "MANTLE_ENV_"
	for _, e := range os.Environ() {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 && strings.HasPrefix(parts[0], prefix) {
			key := strings.TrimPrefix(parts[0], prefix)
			if _, exists := env[key]; exists {
				slog.Info("env variable overrides config", "key", prefix+key, "config_key", "env."+key)
			}
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
