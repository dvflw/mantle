# env: Map in mantle.yaml — Design Spec

**Issue:** #74
**Milestone:** v0.5.0 — The GitOps Update

---

## Summary

Add an optional `env:` section to `mantle.yaml` that populates the `env.*` namespace in CEL expressions. Merges with `MANTLE_ENV_*` shell environment variables at runtime, with env vars taking precedence. Logs when an env var overrides a YAML value.

## Schema

```yaml
version: 1
env:
  SLACK_CHANNEL: "#ops-alerts"
  ENVIRONMENT: production
  NOTIFY_EMAIL: "team@example.com"
database:
  url: postgres://...
```

Values are accessible in CEL as `env.SLACK_CHANNEL`, `env.ENVIRONMENT`, etc. — same namespace as existing `MANTLE_ENV_*` variables.

## Merge Behavior

1. Start with `cfg.Env` map from mantle.yaml
2. Overlay `MANTLE_ENV_*` from `os.Environ()` (strip prefix as today)
3. When a key exists in both, the env var wins and an `slog.Info` is emitted: `"env variable MANTLE_ENV_<KEY> overrides config env.<KEY>"`

## Changes

### `internal/config/config.go`

Add field to `Config` struct:

```go
Env map[string]string `mapstructure:"env"`
```

No Viper env binding — the `env:` section is a free-form map read from the unmarshaled struct only. No `v.SetDefault` needed (nil map is fine — merge handles it).

### `internal/cel/cel.go`

Change `envVars()` to accept the config env map:

```go
func envVars(configEnv map[string]string) map[string]string
```

1. Copy `configEnv` into result map (nil-safe)
2. Iterate `os.Environ()` for `MANTLE_ENV_*` prefix
3. For each match, check if key already exists in result — if so, `slog.Info` override message
4. Set the value (env var wins)

Update the `NewEvaluator` constructor (or add a method) to accept the config env map and pass it to `envVars()`. The evaluator caches env vars at construction time, so the config map is needed at that point.

### Engine wiring

Where the CEL evaluator is created (in `engine.New()` or the CLI commands that construct engines), pass `cfg.Env` to the evaluator. This may require:
- Adding a `ConfigEnv` parameter to `NewEvaluator`, or
- Adding a `WithConfigEnv(map[string]string)` option, or
- Passing it through the `Engine` struct

Given that `engine.New()` currently takes only `*sql.DB`, the simplest approach is to update `NewEvaluator` to accept the config env map: `NewEvaluator(configEnv map[string]string)`. Then pass `cfg.Env` at each call site where the evaluator is created (CLI commands and serve). This avoids changing the `Engine.New()` signature.

### Documentation

Update `packages/site/src/content/docs/configuration.md`:
- Document the `env:` section
- Explain merge precedence (env vars > YAML)
- Example showing override behavior

### Tests

- `cel_test.go`: Test that config env values appear in CEL context
- `cel_test.go`: Test that `MANTLE_ENV_*` overrides config env
- `config_test.go`: Test that `env:` map parses from YAML

## What This Does NOT Do

- No per-workflow env scoping (engine-wide only)
- No env var binding for individual `env.*` keys via Viper
- No secret support in `env:` values (use `credential:` for secrets)
- No validation of env key names (free-form strings)
