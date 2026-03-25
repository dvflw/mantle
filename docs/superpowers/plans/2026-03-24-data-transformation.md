# Data Transformation CEL Functions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add custom CEL functions (string, type coercion, object construction, JSON, date/time, collections, null coalescing) and comprehensive documentation for data transformation patterns.

**Architecture:** Custom functions are registered as `cel.Function` options in `cel.NewEnv()`. A new `functions.go` file defines all functions; `cel.go` is modified only to pass them through. Documentation covers both the new functions and the already-working-but-undocumented macros (`.map()`, `.filter()`, etc.).

**Tech Stack:** Go, cel-go v0.27.0 (`cel.Function`, `cel.Overload`, `cel.UnaryBinding`/`cel.BinaryBinding`/`cel.FunctionBinding`), `encoding/json`, `time`

**Spec:** `docs/superpowers/specs/2026-03-24-data-transformation-design.md`

---

## Task 1: Test and document built-in macros

**Files:**
- Create: `internal/cel/macros_test.go`

These macros already work but have no tests. This task locks in their behavior.

- [ ] **Step 1: Write tests for built-in macros**

In `internal/cel/macros_test.go`:

```go
package cel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newListContext() *Context {
	return &Context{
		Steps: map[string]map[string]any{
			"fetch": {
				"output": map[string]any{
					"items": []any{
						map[string]any{"name": "alice", "age": int64(30)},
						map[string]any{"name": "bob", "age": int64(17)},
						map[string]any{"name": "charlie", "age": int64(25)},
					},
				},
			},
		},
		Inputs: map[string]any{},
	}
}

func TestMacro_Map(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	result, err := eval.Eval(`steps.fetch.output.items.map(item, item.name)`, newListContext())
	require.NoError(t, err)
	assert.Equal(t, []any{"alice", "bob", "charlie"}, result)
}

func TestMacro_Filter(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	result, err := eval.Eval(`steps.fetch.output.items.filter(item, item.age >= 21)`, newListContext())
	require.NoError(t, err)

	items, ok := result.([]any)
	require.True(t, ok)
	assert.Len(t, items, 2)
}

func TestMacro_Exists(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	result, err := eval.Eval(`steps.fetch.output.items.exists(item, item.name == "bob")`, newListContext())
	require.NoError(t, err)
	assert.Equal(t, true, result)

	result, err = eval.Eval(`steps.fetch.output.items.exists(item, item.name == "dave")`, newListContext())
	require.NoError(t, err)
	assert.Equal(t, false, result)
}

func TestMacro_All(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	result, err := eval.Eval(`steps.fetch.output.items.all(item, item.age > 0)`, newListContext())
	require.NoError(t, err)
	assert.Equal(t, true, result)

	result, err = eval.Eval(`steps.fetch.output.items.all(item, item.age >= 21)`, newListContext())
	require.NoError(t, err)
	assert.Equal(t, false, result)
}

func TestMacro_ExistsOne(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	result, err := eval.Eval(`steps.fetch.output.items.exists_one(item, item.name == "alice")`, newListContext())
	require.NoError(t, err)
	assert.Equal(t, true, result)
}

func TestMacro_MapAndFilter_Chained(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	result, err := eval.Eval(`steps.fetch.output.items.filter(item, item.age >= 21).map(item, item.name)`, newListContext())
	require.NoError(t, err)
	assert.Equal(t, []any{"alice", "charlie"}, result)
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test ./internal/cel/ -run "TestMacro_" -v`
Expected: PASS — all macros already work, we're just adding coverage

- [ ] **Step 3: Commit**

```bash
git add internal/cel/macros_test.go
git commit -m "test(cel): add coverage for built-in map/filter/exists/all macros"
```

---

## Task 2: String functions — toLower, toUpper, trim

**Files:**
- Create: `internal/cel/functions.go`
- Create: `internal/cel/functions_test.go`
- Modify: `internal/cel/cel.go:30-36`

- [ ] **Step 1: Write failing tests**

In `internal/cel/functions_test.go`:

```go
package cel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunc_ToLower(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	tests := []struct {
		name  string
		expr  string
		want  any
	}{
		{"basic", `"HELLO".toLower()`, "hello"},
		{"mixed", `"Hello World".toLower()`, "hello world"},
		{"already_lower", `"hello".toLower()`, "hello"},
		{"empty", `"".toLower()`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := eval.Eval(tt.expr, newTestContext())
			require.NoError(t, err)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestFunc_ToUpper(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	tests := []struct {
		name string
		expr string
		want any
	}{
		{"basic", `"hello".toUpper()`, "HELLO"},
		{"mixed", `"Hello World".toUpper()`, "HELLO WORLD"},
		{"empty", `"".toUpper()`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := eval.Eval(tt.expr, newTestContext())
			require.NoError(t, err)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestFunc_Trim(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	tests := []struct {
		name string
		expr string
		want any
	}{
		{"spaces", `"  hello  ".trim()`, "hello"},
		{"tabs", "\"\\thello\\t\".trim()", "hello"},
		{"no_whitespace", `"hello".trim()`, "hello"},
		{"empty", `"".trim()`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := eval.Eval(tt.expr, newTestContext())
			require.NoError(t, err)
			assert.Equal(t, tt.want, result)
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cel/ -run "TestFunc_ToLower|TestFunc_ToUpper|TestFunc_Trim" -v`
Expected: FAIL — functions not registered

- [ ] **Step 3: Create functions.go with string functions and wire into cel.go**

In `internal/cel/functions.go`:

```go
package cel

import (
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

// customFunctions returns all custom CEL function options for the Mantle environment.
func customFunctions() []cel.EnvOption {
	return []cel.EnvOption{
		stringFunctions(),
	}
}

func stringFunctions() cel.EnvOption {
	return cel.Lib(&stringLib{})
}

type stringLib struct{}

func (l *stringLib) CompileOptions() []cel.EnvOption {
	return []cel.EnvOption{
		cel.Function("toLower",
			cel.MemberOverload("string_toLower",
				[]*cel.Type{cel.StringType},
				cel.StringType,
				cel.UnaryBinding(func(val ref.Val) ref.Val {
					return types.String(strings.ToLower(string(val.(types.String))))
				}),
			),
		),
		cel.Function("toUpper",
			cel.MemberOverload("string_toUpper",
				[]*cel.Type{cel.StringType},
				cel.StringType,
				cel.UnaryBinding(func(val ref.Val) ref.Val {
					return types.String(strings.ToUpper(string(val.(types.String))))
				}),
			),
		),
		cel.Function("trim",
			cel.MemberOverload("string_trim",
				[]*cel.Type{cel.StringType},
				cel.StringType,
				cel.UnaryBinding(func(val ref.Val) ref.Val {
					return types.String(strings.TrimSpace(string(val.(types.String))))
				}),
			),
		),
	}
}

func (l *stringLib) ProgramOptions() []cel.ProgramOption {
	return nil
}
```

In `internal/cel/cel.go`, update `NewEvaluator` (lines 30-36) to include custom functions:

```go
func NewEvaluator() (*Evaluator, error) {
	opts := []cel.EnvOption{
		cel.Variable("steps", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("inputs", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("env", cel.MapType(cel.StringType, cel.StringType)),
		cel.Variable("trigger", cel.MapType(cel.StringType, cel.DynType)),
	}
	opts = append(opts, customFunctions()...)

	env, err := cel.NewEnv(opts...)
	if err != nil {
		return nil, fmt.Errorf("creating CEL environment: %w", err)
	}
	return &Evaluator{env: env, envCache: envVars()}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cel/ -run "TestFunc_ToLower|TestFunc_ToUpper|TestFunc_Trim" -v`
Expected: PASS

- [ ] **Step 5: Run all existing CEL tests to verify no regression**

Run: `go test ./internal/cel/ -v`
Expected: PASS — all existing tests still pass

- [ ] **Step 6: Commit**

```bash
git add internal/cel/functions.go internal/cel/functions_test.go internal/cel/cel.go
git commit -m "feat(cel): add toLower, toUpper, trim string functions"
```

---

## Task 3: String functions — replace and split

**Files:**
- Modify: `internal/cel/functions.go`
- Modify: `internal/cel/functions_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/cel/functions_test.go`:

```go
func TestFunc_Replace(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	tests := []struct {
		name string
		expr string
		want any
	}{
		{"basic", `"foo-bar".replace("-", "_")`, "foo_bar"},
		{"multiple", `"a.b.c".replace(".", "/")`, "a/b/c"},
		{"no_match", `"hello".replace("x", "y")`, "hello"},
		{"empty_replacement", `"hello".replace("l", "")`, "heo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := eval.Eval(tt.expr, newTestContext())
			require.NoError(t, err)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestFunc_Split(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	tests := []struct {
		name string
		expr string
		want any
	}{
		{"comma", `"a,b,c".split(",")`, []any{"a", "b", "c"}},
		{"space", `"hello world".split(" ")`, []any{"hello", "world"}},
		{"no_match", `"hello".split(",")`, []any{"hello"}},
		{"empty_string", `"".split(",")`, []any{""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := eval.Eval(tt.expr, newTestContext())
			require.NoError(t, err)
			assert.Equal(t, tt.want, result)
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cel/ -run "TestFunc_Replace|TestFunc_Split" -v`
Expected: FAIL

- [ ] **Step 3: Add replace and split to stringLib.CompileOptions**

In `functions.go`, add to `stringLib.CompileOptions()`:

```go
		cel.Function("replace",
			cel.MemberOverload("string_replace",
				[]*cel.Type{cel.StringType, cel.StringType, cel.StringType},
				cel.StringType,
				cel.FunctionBinding(func(args ...ref.Val) ref.Val {
					s := string(args[0].(types.String))
					old := string(args[1].(types.String))
					new := string(args[2].(types.String))
					return types.String(strings.ReplaceAll(s, old, new))
				}),
			),
		),
		cel.Function("split",
			cel.MemberOverload("string_split",
				[]*cel.Type{cel.StringType, cel.StringType},
				cel.ListType(cel.StringType),
				cel.BinaryBinding(func(lhs, rhs ref.Val) ref.Val {
					s := string(lhs.(types.String))
					sep := string(rhs.(types.String))
					parts := strings.Split(s, sep)
					return types.DefaultTypeAdapter.NativeToValue(parts)
				}),
			),
		),
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cel/ -run "TestFunc_Replace|TestFunc_Split" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/cel/functions.go internal/cel/functions_test.go
git commit -m "feat(cel): add replace and split string functions"
```

---

## Task 4: Type coercion — parseInt, parseFloat, toString

**Files:**
- Modify: `internal/cel/functions.go`
- Modify: `internal/cel/functions_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/cel/functions_test.go`:

```go
func TestFunc_ParseInt(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	tests := []struct {
		name    string
		expr    string
		want    any
		wantErr bool
	}{
		{"valid", `parseInt("42")`, int64(42), false},
		{"negative", `parseInt("-7")`, int64(-7), false},
		{"zero", `parseInt("0")`, int64(0), false},
		{"invalid", `parseInt("abc")`, nil, true},
		{"float_string", `parseInt("3.14")`, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := eval.Eval(tt.expr, newTestContext())
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, result)
			}
		})
	}
}

func TestFunc_ParseFloat(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	tests := []struct {
		name    string
		expr    string
		want    any
		wantErr bool
	}{
		{"valid", `parseFloat("3.14")`, 3.14, false},
		{"integer", `parseFloat("42")`, 42.0, false},
		{"negative", `parseFloat("-1.5")`, -1.5, false},
		{"invalid", `parseFloat("abc")`, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := eval.Eval(tt.expr, newTestContext())
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, result)
			}
		})
	}
}

func TestFunc_ToString(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	tests := []struct {
		name string
		expr string
		want any
	}{
		{"int", `toString(42)`, "42"},
		{"bool", `toString(true)`, "true"},
		{"string", `toString("hello")`, "hello"},
		{"float", `toString(3.14)`, "3.14"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := eval.Eval(tt.expr, newTestContext())
			require.NoError(t, err)
			assert.Equal(t, tt.want, result)
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cel/ -run "TestFunc_ParseInt|TestFunc_ParseFloat|TestFunc_ToString" -v`
Expected: FAIL

- [ ] **Step 3: Add type coercion functions**

In `functions.go`, add a new library and register it in `customFunctions()`:

```go
func typeFunctions() cel.EnvOption {
	return cel.Lib(&typeLib{})
}

type typeLib struct{}

func (l *typeLib) CompileOptions() []cel.EnvOption {
	return []cel.EnvOption{
		cel.Function("parseInt",
			cel.Overload("parseInt_string",
				[]*cel.Type{cel.StringType},
				cel.IntType,
				cel.UnaryBinding(func(val ref.Val) ref.Val {
					s := string(val.(types.String))
					n, err := strconv.ParseInt(s, 10, 64)
					if err != nil {
						return types.NewErr("parseInt: %v", err)
					}
					return types.Int(n)
				}),
			),
		),
		cel.Function("parseFloat",
			cel.Overload("parseFloat_string",
				[]*cel.Type{cel.StringType},
				cel.DoubleType,
				cel.UnaryBinding(func(val ref.Val) ref.Val {
					s := string(val.(types.String))
					f, err := strconv.ParseFloat(s, 64)
					if err != nil {
						return types.NewErr("parseFloat: %v", err)
					}
					return types.Double(f)
				}),
			),
		),
		cel.Function("toString",
			cel.Overload("toString_any",
				[]*cel.Type{cel.DynType},
				cel.StringType,
				cel.UnaryBinding(func(val ref.Val) ref.Val {
					return types.String(fmt.Sprintf("%v", val.Value()))
				}),
			),
		),
	}
}

func (l *typeLib) ProgramOptions() []cel.ProgramOption {
	return nil
}
```

Add `"fmt"` and `"strconv"` to imports. Update `customFunctions()`:

```go
func customFunctions() []cel.EnvOption {
	return []cel.EnvOption{
		stringFunctions(),
		typeFunctions(),
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cel/ -run "TestFunc_ParseInt|TestFunc_ParseFloat|TestFunc_ToString" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/cel/functions.go internal/cel/functions_test.go
git commit -m "feat(cel): add parseInt, parseFloat, toString type coercion functions"
```

---

## Task 5: Object construction — obj()

> **Implementation note:** The plan originally specified a variadic `obj()` overload, but cel-go does not support true variadic functions without macros. The implementation uses fixed-arity overloads for 2, 4, 6, 8, and 10 arguments (1–5 key-value pairs), all sharing a single `objBinding` function.

**Files:**
- Modify: `internal/cel/functions.go`
- Modify: `internal/cel/functions_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/cel/functions_test.go`:

```go
func TestFunc_Obj(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	tests := []struct {
		name    string
		expr    string
		want    any
		wantErr bool
	}{
		{
			"basic",
			`obj("name", "alice", "age", 30)`,
			map[string]any{"name": "alice", "age": int64(30)},
			false,
		},
		{
			"single_pair",
			`obj("key", "value")`,
			map[string]any{"key": "value"},
			false,
		},
		{
			"nested_with_step",
			`obj("status", steps.fetch.output.status)`,
			map[string]any{"status": int64(200)},
			false,
		},
		{
			"odd_args",
			`obj("key")`,
			nil,
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := eval.Eval(tt.expr, newTestContext())
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, result)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cel/ -run TestFunc_Obj -v`
Expected: FAIL

- [ ] **Step 3: Add obj function**

In `functions.go`, add:

```go
func collectionFunctions() cel.EnvOption {
	return cel.Lib(&collectionLib{})
}

type collectionLib struct{}

func (l *collectionLib) CompileOptions() []cel.EnvOption {
	return []cel.EnvOption{
		// obj() — register fixed-arity overloads for 1–5 key-value pairs.
		// CEL does not support true variadic functions, so we register
		// overloads for 2/4/6/8/10 args sharing a common objBinding helper.
		cel.Function("obj",
			cel.Overload("obj_2",
				[]*cel.Type{cel.DynType, cel.DynType},
				cel.DynType, cel.FunctionBinding(objBinding)),
			cel.Overload("obj_4",
				[]*cel.Type{cel.DynType, cel.DynType, cel.DynType, cel.DynType},
				cel.DynType, cel.FunctionBinding(objBinding)),
			cel.Overload("obj_6",
				[]*cel.Type{cel.DynType, cel.DynType, cel.DynType, cel.DynType, cel.DynType, cel.DynType},
				cel.DynType, cel.FunctionBinding(objBinding)),
			cel.Overload("obj_8",
				[]*cel.Type{cel.DynType, cel.DynType, cel.DynType, cel.DynType, cel.DynType, cel.DynType, cel.DynType, cel.DynType},
				cel.DynType, cel.FunctionBinding(objBinding)),
			cel.Overload("obj_10",
				[]*cel.Type{cel.DynType, cel.DynType, cel.DynType, cel.DynType, cel.DynType, cel.DynType, cel.DynType, cel.DynType, cel.DynType, cel.DynType},
				cel.DynType, cel.FunctionBinding(objBinding)),
		),
	}
}

func (l *collectionLib) ProgramOptions() []cel.ProgramOption {
	return nil
}
```

Update `customFunctions()`:

```go
func customFunctions() []cel.EnvOption {
	return []cel.EnvOption{
		stringFunctions(),
		typeFunctions(),
		collectionFunctions(),
	}
}
```

Note: `refToNative` is defined in `cel.go` and accessible since both files are in the same package.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cel/ -run TestFunc_Obj -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/cel/functions.go internal/cel/functions_test.go
git commit -m "feat(cel): add obj() map construction function"
```

---

## Task 6: Utility functions — default, flatten

**Files:**
- Modify: `internal/cel/functions.go`
- Modify: `internal/cel/functions_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/cel/functions_test.go`:

```go
func TestFunc_Default(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	// Test with a value that exists.
	result, err := eval.Eval(`default(inputs.url, "fallback")`, newTestContext())
	require.NoError(t, err)
	assert.Equal(t, "https://example.com", result)

	// Test with a missing key — CEL map access on missing key errors,
	// so default should catch that. We test with a direct null/0 fallback.
	result, err = eval.Eval(`default("", "fallback")`, newTestContext())
	require.NoError(t, err)
	assert.Equal(t, "", result) // empty string is not null, returns as-is
}

func TestFunc_Flatten(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	ctx := &Context{
		Steps: map[string]map[string]any{
			"data": {
				"output": map[string]any{
					"nested": []any{
						[]any{int64(1), int64(2)},
						[]any{int64(3), int64(4)},
					},
				},
			},
		},
		Inputs: map[string]any{},
	}

	result, err := eval.Eval(`flatten(steps.data.output.nested)`, ctx)
	require.NoError(t, err)
	assert.Equal(t, []any{int64(1), int64(2), int64(3), int64(4)}, result)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cel/ -run "TestFunc_Default|TestFunc_Flatten" -v`
Expected: FAIL

- [ ] **Step 3: Add default and flatten to collectionLib**

In `functions.go`, add to `collectionLib.CompileOptions()`:

```go
		cel.Function("default",
			cel.Overload("default_any_any",
				[]*cel.Type{cel.DynType, cel.DynType},
				cel.DynType,
				cel.OverloadIsNonStrict(),
				cel.BinaryBinding(func(lhs, rhs ref.Val) ref.Val {
					if types.IsError(lhs) || types.IsUnknown(lhs) || lhs == types.NullValue {
						return rhs
					}
					return lhs
				}),
			),
		),
		cel.Function("flatten",
			cel.Overload("flatten_list",
				[]*cel.Type{cel.ListType(cel.DynType)},
				cel.ListType(cel.DynType),
				cel.UnaryBinding(func(val ref.Val) ref.Val {
					list := val.(traits.Lister)
					var result []any
					it := list.Iterator()
					for it.HasNext() == types.True {
						item := it.Next()
						if sub, ok := item.(traits.Lister); ok {
							subIt := sub.Iterator()
							for subIt.HasNext() == types.True {
								result = append(result, refToNative(subIt.Next()))
							}
						} else {
							result = append(result, refToNative(item))
						}
					}
					return types.DefaultTypeAdapter.NativeToValue(result)
				}),
			),
		),
```

Add a helper function in `functions.go`:

```go
func nativeSlice(vals []ref.Val) []any {
	result := make([]any, len(vals))
	for i, v := range vals {
		result[i] = refToNative(v)
	}
	return result
}
```

Add `"github.com/google/cel-go/common/types/traits"` to imports.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cel/ -run "TestFunc_Default|TestFunc_Flatten" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/cel/functions.go internal/cel/functions_test.go
git commit -m "feat(cel): add default() null coalescing and flatten() functions"
```

---

## Task 7: JSON functions — jsonEncode, jsonDecode

**Files:**
- Modify: `internal/cel/functions.go`
- Modify: `internal/cel/functions_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/cel/functions_test.go`:

```go
func TestFunc_JsonEncode(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	result, err := eval.Eval(`jsonEncode(obj("name", "alice", "age", 30))`, newTestContext())
	require.NoError(t, err)

	// JSON key order may vary, so parse and compare.
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(result.(string)), &parsed))
	assert.Equal(t, "alice", parsed["name"])
	assert.Equal(t, float64(30), parsed["age"]) // JSON numbers decode as float64
}

func TestFunc_JsonDecode(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	tests := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{"object", `jsonDecode("{\"name\":\"alice\"}")`, false},
		{"array", `jsonDecode("[1,2,3]")`, false},
		{"invalid", `jsonDecode("not json")`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := eval.Eval(tt.expr, newTestContext())
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
```

Add `"encoding/json"` to test file imports.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cel/ -run "TestFunc_Json" -v`
Expected: FAIL

- [ ] **Step 3: Add JSON functions**

In `functions.go`, add a new library:

```go
func jsonFunctions() cel.EnvOption {
	return cel.Lib(&jsonLib{})
}

type jsonLib struct{}

func (l *jsonLib) CompileOptions() []cel.EnvOption {
	return []cel.EnvOption{
		cel.Function("jsonEncode",
			cel.Overload("jsonEncode_any",
				[]*cel.Type{cel.DynType},
				cel.StringType,
				cel.UnaryBinding(func(val ref.Val) ref.Val {
					native := refToNative(val)
					b, err := json.Marshal(native)
					if err != nil {
						return types.NewErr("jsonEncode: %v", err)
					}
					return types.String(string(b))
				}),
			),
		),
		cel.Function("jsonDecode",
			cel.Overload("jsonDecode_string",
				[]*cel.Type{cel.StringType},
				cel.DynType,
				cel.UnaryBinding(func(val ref.Val) ref.Val {
					s := string(val.(types.String))
					dec := json.NewDecoder(strings.NewReader(s))
					dec.UseNumber()
					var result any
					if err := dec.Decode(&result); err != nil {
						return types.NewErr("jsonDecode: %v", err)
					}
					// Reject trailing data by attempting a second decode — must hit EOF.
					var trailing json.RawMessage
					if err := dec.Decode(&trailing); err != io.EOF {
						return types.NewErr("jsonDecode: unexpected trailing data after JSON value")
					}
					return types.DefaultTypeAdapter.NativeToValue(normalizeJSONNumbers(result))
				}),
			),
		),
	}
}

func (l *jsonLib) ProgramOptions() []cel.ProgramOption {
	return nil
}
```

Add `"encoding/json"` to `functions.go` imports. Update `customFunctions()`:

```go
func customFunctions() []cel.EnvOption {
	return []cel.EnvOption{
		stringFunctions(),
		typeFunctions(),
		collectionFunctions(),
		jsonFunctions(),
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cel/ -run "TestFunc_Json" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/cel/functions.go internal/cel/functions_test.go
git commit -m "feat(cel): add jsonEncode and jsonDecode functions"
```

---

## Task 8: Date/time functions — parseTimestamp, formatTimestamp

**Files:**
- Modify: `internal/cel/functions.go`
- Modify: `internal/cel/functions_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/cel/functions_test.go`:

```go
func TestFunc_ParseTimestamp(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	tests := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{"iso8601", `parseTimestamp("2026-03-24T19:00:00Z")`, false},
		{"with_offset", `parseTimestamp("2026-03-24T14:00:00-05:00")`, false},
		{"invalid", `parseTimestamp("not a date")`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := eval.Eval(tt.expr, newTestContext())
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFunc_FormatTimestamp(t *testing.T) {
	eval, err := NewEvaluator()
	require.NoError(t, err)

	result, err := eval.Eval(`formatTimestamp(parseTimestamp("2026-03-24T19:00:00Z"), "2006-01-02")`, newTestContext())
	require.NoError(t, err)
	assert.Equal(t, "2026-03-24", result)

	result, err = eval.Eval(`formatTimestamp(parseTimestamp("2026-03-24T19:30:45Z"), "15:04:05")`, newTestContext())
	require.NoError(t, err)
	assert.Equal(t, "19:30:45", result)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cel/ -run "TestFunc_ParseTimestamp|TestFunc_FormatTimestamp" -v`
Expected: FAIL — functions not registered yet

- [ ] **Step 3: Add date/time functions**

In `functions.go`, add:

```go
func timeFunctions() cel.EnvOption {
	return cel.Lib(&timeLib{})
}

type timeLib struct{}

func (l *timeLib) CompileOptions() []cel.EnvOption {
	return []cel.EnvOption{
		cel.Function("parseTimestamp",
			cel.Overload("parseTimestamp_string",
				[]*cel.Type{cel.StringType},
				cel.TimestampType,
				cel.UnaryBinding(func(val ref.Val) ref.Val {
					s := string(val.(types.String))
					layouts := []string{
						time.RFC3339,
						time.RFC3339Nano,
						"2006-01-02T15:04:05",
						"2006-01-02",
						"01/02/2006",
						"Jan 2, 2006",
					}
					for _, layout := range layouts {
						if t, err := time.Parse(layout, s); err == nil {
							return types.Timestamp{Time: t}
						}
					}
					return types.NewErr("parseTimestamp: unable to parse %q (tried RFC3339, ISO 8601 date, and common formats)", s)
				}),
			),
		),
		cel.Function("formatTimestamp",
			cel.Overload("formatTimestamp_timestamp_string",
				[]*cel.Type{cel.TimestampType, cel.StringType},
				cel.StringType,
				cel.BinaryBinding(func(lhs, rhs ref.Val) ref.Val {
					ts := lhs.(types.Timestamp)
					layout := string(rhs.(types.String))
					return types.String(ts.Time.Format(layout))
				}),
			),
		),
	}
}

func (l *timeLib) ProgramOptions() []cel.ProgramOption {
	return nil
}
```

Add `"time"` to imports. Update `customFunctions()`:

```go
func customFunctions() []cel.EnvOption {
	return []cel.EnvOption{
		stringFunctions(),
		typeFunctions(),
		collectionFunctions(),
		jsonFunctions(),
		timeFunctions(),
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cel/ -run "TestFunc_Timestamp|TestFunc_FormatTimestamp" -v`
Expected: PASS

- [ ] **Step 5: Run full test suite**

Run: `go test ./internal/cel/ -v`
Expected: PASS — all tests including existing ones

- [ ] **Step 6: Commit**

```bash
git add internal/cel/functions.go internal/cel/functions_test.go
git commit -m "feat(cel): add parseTimestamp and formatTimestamp date/time functions"
```

---

## Task 9: Update CEL expressions documentation

**Files:**
- Modify: `site/src/content/docs/concepts/expressions.md`

This task is delegated to the technical writer agent.

- [ ] **Step 1: Read the current expressions.md**

Read: `site/src/content/docs/concepts/expressions.md`

- [ ] **Step 2: Add new sections for custom functions and macros**

After the existing content, add sections covering:

**Built-in List Macros:**
- `.map(item, expr)` — transform each element
- `.filter(item, expr)` — keep matching elements
- `.exists(item, expr)` — true if any match
- `.all(item, expr)` — true if all match
- `.exists_one(item, expr)` — true if exactly one matches
- Chaining example: `.filter(...).map(...)`

**String Functions:**
- `toLower()`, `toUpper()`, `trim()`, `replace(old, new)`, `split(delim)`

**Type Coercion:**
- `parseInt(string)`, `parseFloat(string)`, `toString(any)`

**Object Construction:**
- `obj(key, value, ...)` with usage examples for building params maps

**Utility Functions:**
- `default(value, fallback)`
- `flatten(list)`

**JSON Functions:**
- `jsonEncode(value)`, `jsonDecode(string)`

**Date/Time Functions:**
- `parseTimestamp(string)`, `formatTimestamp(ts, layout)` with Go layout reference

Each function should have a brief description and a YAML example showing usage in a workflow step.

- [ ] **Step 3: Verify site builds**

Run: `cd site && npm run build`
Expected: success

- [ ] **Step 4: Commit**

```bash
git add site/src/content/docs/concepts/expressions.md
git commit -m "docs: add custom CEL functions and macros to expressions reference (#14)"
```

---

## Task 10: Create data transformations guide

**Files:**
- Create: `site/src/content/docs/getting-started/data-transformations.md`

This task is delegated to the technical writer agent.

- [ ] **Step 1: Create the guide**

Write `site/src/content/docs/getting-started/data-transformations.md` covering three patterns:

**Pattern 1 — Structural transforms (CEL only):**
Complete workflow example: fetch user list from API → `.map()` + `obj()` to reshape each record → Postgres INSERT. Show the full YAML with CEL expressions in params.

**Pattern 2 — AI-powered transforms:**
Complete workflow example: fetch raw text/HTML → AI connector with `output_schema` to extract structured data → store results. Explain when to use AI vs CEL (interpretation vs reshaping).

**Pattern 3 — Hybrid:**
Complete workflow example: fetch data → CEL for field extraction and normalization → AI for classification/enrichment → Postgres store. Show how to combine both approaches.

Include a decision guide: "Use CEL when the mapping is known and structural. Use AI when the transform requires interpretation, classification, or natural language understanding."

- [ ] **Step 2: Verify site builds**

Run: `cd site && npm run build`
Expected: success

- [ ] **Step 3: Commit**

```bash
git add site/src/content/docs/getting-started/data-transformations.md
git commit -m "docs: add data transformation patterns guide (#14)"
```

---

## Task 11: Create example workflows

**Files:**
- Create: `examples/data-transform-api-to-db.yaml`
- Create: `examples/ai-data-enrichment.yaml`

- [ ] **Step 1: Create structural transform example**

In `examples/data-transform-api-to-db.yaml`:

```yaml
name: data-transform-api-to-db
description: >
  Fetches a user from an API, transforms the record using CEL expressions
  to match a database schema, and inserts the normalized data into Postgres.
  Demonstrates toLower() and string functions without requiring an AI model.

steps:
  - name: fetch-user
    action: http/request
    timeout: "15s"
    params:
      method: GET
      url: "https://jsonplaceholder.typicode.com/users/1"
      headers:
        Accept: "application/json"

  - name: store-user
    action: postgres/query
    credential: app-db
    params:
      query: "INSERT INTO users (username, email, city) VALUES ($1, $2, $3)"
      args:
        - "{{ steps['fetch-user'].output.json.username.toLower() }}"
        - "{{ steps['fetch-user'].output.json.email.toLower() }}"
        - "{{ steps['fetch-user'].output.json.address.city }}"
```

- [ ] **Step 2: Create AI enrichment example**

In `examples/ai-data-enrichment.yaml`:

```yaml
name: ai-data-enrichment
description: >
  Fetches support tickets, uses an AI model to classify priority and
  extract key entities, then stores the enriched data. Demonstrates
  using AI for transforms that require interpretation rather than
  simple structural mapping.

inputs:
  ticket_api_url:
    type: string
    description: URL to fetch support tickets from

steps:
  - name: fetch-tickets
    action: http/request
    timeout: "15s"
    params:
      method: GET
      url: "{{ inputs.ticket_api_url }}"
      headers:
        Accept: "application/json"

  - name: classify
    action: ai/completion
    credential: openai
    timeout: "60s"
    params:
      model: gpt-4o
      system_prompt: >
        You are a support ticket classifier. Given a ticket, determine
        the priority (critical, high, medium, low), category, and extract
        any mentioned product names or error codes.
      prompt: "Classify this ticket: {{ steps['fetch-tickets'].output.body }}"
      output_schema:
        type: object
        properties:
          priority:
            type: string
            enum: [critical, high, medium, low]
          category:
            type: string
          products:
            type: array
            items:
              type: string
          error_codes:
            type: array
            items:
              type: string
        required: [priority, category, products, error_codes]
        additionalProperties: false

  - name: store-enriched
    action: postgres/query
    credential: app-db
    if: "steps.classify.output.json.priority == 'critical' || steps.classify.output.json.priority == 'high'"
    params:
      query: >
        INSERT INTO urgent_tickets (priority, category, products, raw_body)
        VALUES ($1, $2, $3, $4)
      args:
        - "{{ steps.classify.output.json.priority }}"
        - "{{ steps.classify.output.json.category }}"
        - "{{ jsonEncode(steps.classify.output.json.products) }}"
        - "{{ steps['fetch-tickets'].output.body }}"
```

- [ ] **Step 3: Commit**

```bash
git add examples/data-transform-api-to-db.yaml examples/ai-data-enrichment.yaml
git commit -m "feat: add data transformation and AI enrichment example workflows (#14)"
```

---

## Task 12: Final validation

- [ ] **Step 1: Run full test suite**

Run: `go test ./internal/cel/ -v`
Expected: PASS — all function tests, macro tests, and existing tests

- [ ] **Step 2: Run go vet and golangci-lint**

Run: `go vet ./internal/cel/`
Expected: clean

Run: `golangci-lint run ./...`
Expected: clean

- [ ] **Step 3: Verify site builds**

Run: `cd site && npm run build`
Expected: success

- [ ] **Step 4: Run full project test suite**

Run: `go test ./... -short`
Expected: PASS
