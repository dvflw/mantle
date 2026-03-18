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

```
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
3. Call `AutomaticEnv()` with `MANTLE_` prefix and `_` as the nested key delimiter
4. Unmarshal into `Config` struct and return

## Override Precedence

Viper's built-in precedence (highest to lowest):
1. CLI flags (bound to Viper keys via Cobra)
2. Environment variables (`MANTLE_` prefix)
3. Config file (`mantle.yaml`)
4. Defaults

## Cobra Integration

In `internal/cli/root.go`:
- `PersistentPreRunE` on the root command calls `config.Load()` with the `--config` flag value
- The resulting `*Config` is stored on the command's context
- Subcommands retrieve config from context

## Dependencies

- `github.com/spf13/viper` — config loading, env var binding, YAML parsing

## What's NOT Included

- Config validation beyond type checking
- Hot reload / file watching
- Config fields beyond database.url, api.address, log.level (added by future phases)
