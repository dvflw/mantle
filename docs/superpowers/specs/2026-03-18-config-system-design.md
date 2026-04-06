# Design: Config System — mantle.yaml and CLI Flags

> Linear issue: [DVFLW-272](https://linear.app/dvflw/issue/DVFLW-272/config-system-mantleyaml-and-cli-flags)
> Date: 2026-03-18

## Goal

Add a configuration system using Viper that loads settings from `mantle.yaml`, with environment variable and CLI flag overrides.

## Acceptance Criteria

- `mantle.yaml` supports Postgres connection string, API listen address, log level
- CLI flags override config file values
- Env vars (e.g., `MANTLE_DATABASE_URL`) override both config file and defaults
- Config file path configurable via `--config` flag

## Package Structure

```text
internal/
  config/
    config.go        # Config struct, Load() function, Viper setup
    config_test.go   # Tests for loading, env overrides, flag overrides, defaults
```

## Config Struct

```go
type Config struct {
    Database DatabaseConfig `mapstructure:"database"`
    API      APIConfig      `mapstructure:"api"`
    Log      LogConfig      `mapstructure:"log"`
}

type DatabaseConfig struct {
    URL string `mapstructure:"url"`
}

type APIConfig struct {
    Address string `mapstructure:"address"`
}

type LogConfig struct {
    Level string `mapstructure:"level"`
}
```

## Default Values

| Key | Default | Env Var |
|-----|---------|---------|
| `database.url` | `postgres://localhost:5432/mantle?sslmode=disable` | `MANTLE_DATABASE_URL` |
| `api.address` | `:8080` | `MANTLE_API_ADDRESS` |
| `log.level` | `info` | `MANTLE_LOG_LEVEL` |

## Example mantle.yaml

```yaml
database:
  url: "postgres://localhost:5432/mantle?sslmode=disable"

api:
  address: ":8080"

log:
  level: "info"
```

## Load Function

`Load(configPath string) (*Config, error)` — takes the `--config` flag value.

Steps:
1. Set defaults for all keys
2. If `configPath` is provided, read that file; otherwise search current directory for `mantle.yaml`
3. Set `MANTLE_` env prefix via `SetEnvPrefix("MANTLE")`
4. Explicitly bind env vars with `BindEnv` for each key (not `AutomaticEnv`, which cannot reliably resolve nested keys with underscore delimiters):
   - `viper.BindEnv("database.url", "MANTLE_DATABASE_URL")`
   - `viper.BindEnv("api.address", "MANTLE_API_ADDRESS")`
   - `viper.BindEnv("log.level", "MANTLE_LOG_LEVEL")`
5. Unmarshal into `Config` struct and return

### Missing Config File Behavior

- If `--config` is provided and the file does not exist: **hard error** (user explicitly asked for this file)
- If `--config` is not provided and no `mantle.yaml` in current directory: **silent fallback to defaults** (config file is optional)

### Validation

`Load()` returns an error only for YAML parse failures and type mismatches. It does **not** perform semantic validation (e.g., whether `database.url` is a valid Postgres connection string). Semantic validation belongs in the consuming code.

## Override Precedence

Viper's built-in precedence (highest to lowest):
1. CLI flags (bound to Viper keys via `BindPFlag`)
2. Environment variables (`MANTLE_` prefix, explicit `BindEnv` calls)
3. Config file (`mantle.yaml`)
4. Defaults

## CLI Flags

Add these persistent flags on the root command:
- `--database-url` — binds to Viper key `database.url`
- `--api-address` — binds to Viper key `api.address`
- `--log-level` — binds to Viper key `log.level`

Flags are bound to Viper keys via `viper.BindPFlag()` inside `Load()`, which receives the `*cobra.Command` to access its flags. Updated signature:

`Load(cmd *cobra.Command) (*Config, error)` — reads `--config` flag from `cmd`, binds other flags to Viper.

## Cobra Integration

In `internal/cli/root.go`:
- Add `--database-url`, `--api-address`, `--log-level` as persistent flags
- `PersistentPreRunE` on the root command calls `config.Load(cmd)` which reads `--config`, binds flags, and loads config
- The resulting `*Config` is stored on the command's context via `context.WithValue`

### Context Key

Defined in `internal/config/`:

```go
type contextKey struct{}

// WithContext returns a new context with the config attached.
func WithContext(ctx context.Context, cfg *Config) context.Context {
    return context.WithValue(ctx, contextKey{}, cfg)
}

// FromContext retrieves the config from context. Returns nil if not set.
func FromContext(ctx context.Context) *Config {
    cfg, _ := ctx.Value(contextKey{}).(*Config)
    return cfg
}
```

The context key type is unexported, preventing collisions. Helper functions are in the `config` package so subcommands depend on `config` (not the other way around).

## Dependencies

- `github.com/spf13/viper` — config loading, env var binding, YAML parsing (must be added to go.mod)

## What's NOT Included

- Semantic validation of config values (belongs in consuming code)
- Hot reload / file watching
- Config fields beyond database.url, api.address, log.level (added by future phases)
