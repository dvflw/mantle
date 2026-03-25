# Data Transformation — CEL Functions & Documentation

**Date:** 2026-03-24
**Issue:** [#14 — Data Transformation Step](https://github.com/dvflw/mantle/issues/14)
**Status:** Draft

## Problem

Mantle workflows can pass data between steps via CEL expressions, but lack the tools to reshape that data. The common pattern — fetch from API, normalize for a DB schema, store — requires either manual field-by-field construction (not possible in CEL today) or routing through the AI connector (slow, expensive, non-deterministic for structural transforms).

## Discovery: Existing Hidden Capabilities

CEL's default environment includes macros that already work in Mantle but were never documented or tested:

- `.map(item, expr)` — transform each element in a list
- `.filter(item, expr)` — keep elements matching a predicate
- `.exists(item, expr)` — true if any element matches
- `.all(item, expr)` — true if all elements match
- `.exists_one(item, expr)` — true if exactly one matches

These need documentation and tests, not implementation.

## Design

### Custom CEL Functions

All functions registered in `internal/cel/functions.go` via `cel.Function()` options passed to `cel.NewEnv()`. Pure functions, no side effects.

#### String Functions (methods on string type)

| Function | Example | Result |
|----------|---------|--------|
| `toLower()` | `"HELLO".toLower()` | `"hello"` |
| `toUpper()` | `"hello".toUpper()` | `"HELLO"` |
| `trim()` | `"  hello  ".trim()` | `"hello"` |
| `replace(old, new)` | `"foo-bar".replace("-", "_")` | `"foo_bar"` |
| `split(delim)` | `"a,b,c".split(",")` | `["a", "b", "c"]` |

#### Type Coercion (global functions)

| Function | Example | Result |
|----------|---------|--------|
| `parseInt(string)` | `parseInt("42")` | `42` |
| `parseFloat(string)` | `parseFloat("3.14")` | `3.14` |
| `toString(any)` | `toString(42)` | `"42"` |

#### Object Construction (global function)

| Function | Example | Result |
|----------|---------|--------|
| `obj(k1, v1, k2, v2, ...)` | `obj("name", "alice", "age", 30)` | `{"name": "alice", "age": 30}` |

Errors on odd number of args or non-string keys. Enables building maps for DB inserts and API payloads.

#### Null Coalescing (global function)

| Function | Example | Result |
|----------|---------|--------|
| `default(value, fallback)` | `default(steps.x.output.json.name, "unknown")` | value if non-null, else `"unknown"` |

#### JSON (global functions)

| Function | Example | Result |
|----------|---------|--------|
| `jsonEncode(value)` | `jsonEncode(obj("a", 1))` | `'{"a":1}'` |
| `jsonDecode(string)` | `jsonDecode('{"a":1}')` | `{"a": 1}` |

#### Date/Time (global functions)

| Function | Example | Result |
|----------|---------|--------|
| `timestamp(string)` | `timestamp("2026-03-24T19:00:00Z")` | timestamp value |
| `formatTimestamp(ts, layout)` | `formatTimestamp(ts, "2006-01-02")` | `"2026-03-24"` |

Uses Go time layout strings.

#### Collections (global function)

| Function | Example | Result |
|----------|---------|--------|
| `flatten(list)` | `flatten([[1,2],[3,4]])` | `[1,2,3,4]` |

### Integration Point

In `internal/cel/cel.go`, the `NewEvaluator` function passes function options to `cel.NewEnv()`:

```go
func NewEvaluator() (*Evaluator, error) {
    env, err := cel.NewEnv(
        cel.Variable("steps", cel.MapType(cel.StringType, cel.DynType)),
        cel.Variable("inputs", cel.MapType(cel.StringType, cel.DynType)),
        cel.Variable("env", cel.MapType(cel.StringType, cel.StringType)),
        cel.Variable("trigger", cel.MapType(cel.StringType, cel.DynType)),
        // Custom functions
        customFunctions()...,
    )
    // ...
}
```

`customFunctions()` is defined in `functions.go` and returns `[]cel.EnvOption`.

### Error Handling

All errors surface through the existing `Eval` error path:
- Type mismatches: `parseInt("abc")` → evaluation error
- `obj()` with odd args → evaluation error
- `obj()` with non-string keys → evaluation error
- `jsonDecode()` with invalid JSON → evaluation error
- `timestamp()` with unparseable string → evaluation error

No new error types needed.

## Documentation

### CEL Expressions Reference Update

Update `site/src/content/docs/concepts/expressions.md` to add:
- All custom functions organized by category
- The already-working macros (`.map()`, `.filter()`, `.exists()`, `.all()`, `.exists_one()`)
- Examples for each function

### New: Data Transformations Guide

New page at `site/src/content/docs/getting-started/data-transformations.md` covering three patterns:

**Pattern 1 — Structural transforms (CEL only):**
API result → `.map()` + `obj()` → Postgres INSERT. No AI needed. For when the transform is a known schema mapping.

**Pattern 2 — AI-powered transforms:**
Unstructured data → AI connector with `output_schema` → structured output. For when the transform requires interpretation, classification, or natural language understanding.

**Pattern 3 — Hybrid:**
Fetch → CEL for structural normalization → AI for enrichment/classification → Store. Combines both approaches.

Each pattern includes a complete example workflow YAML.

### New Example Workflows

- `examples/data-transform-api-to-db.yaml` — Fetch API → CEL `.map()` + `obj()` → Postgres INSERT (the exact use case from the issue)
- `examples/ai-data-enrichment.yaml` — Fetch data → AI classify/enrich with structured output → store

## Files Changed

### Modified

| File | Change |
|------|--------|
| `internal/cel/cel.go` | Pass `customFunctions()` options to `cel.NewEnv()` |
| `site/src/content/docs/concepts/expressions.md` | Add function reference, document macros |

### New

| File | Purpose |
|------|---------|
| `internal/cel/functions.go` | All custom function definitions |
| `internal/cel/functions_test.go` | Table-driven tests for every custom function |
| `internal/cel/macros_test.go` | Tests for built-in macros (lock in existing behavior) |
| `site/src/content/docs/getting-started/data-transformations.md` | Transformation patterns guide |
| `examples/data-transform-api-to-db.yaml` | Structural transform example workflow |
| `examples/ai-data-enrichment.yaml` | AI transform example workflow |

## Non-Goals

- **Custom user-defined functions** — no plugin/extension API for CEL functions
- **Loops or control flow** — CEL is intentionally non-Turing-complete
- **Regex** — deferring to a future issue; CEL's `matches()` function could be enabled later
- **New connector type** — transformations happen in CEL expressions, not as a separate step type

## Testing Strategy

- **`functions_test.go`** — table-driven: each function gets happy path + error cases (wrong types, empty inputs, edge cases)
- **`macros_test.go`** — tests for `.map()`, `.filter()`, `.exists()`, `.all()`, `.exists_one()` with list data to lock in behavior
- **Existing tests unaffected** — custom functions are additive; no behavior changes to existing expressions
