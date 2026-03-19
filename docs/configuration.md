# Configuration

Mantle loads configuration from three sources: a YAML config file, environment variables, and CLI flags. This page documents every configuration option and how the sources interact.

## Precedence

When the same setting is specified in multiple places, the highest-priority source wins:

1. **CLI flags** (highest priority)
2. **Environment variables**
3. **Config file** (`mantle.yaml`)
4. **Built-in defaults** (lowest priority)

For example, if `mantle.yaml` sets `database.url` to one value and you pass `--database-url` on the command line, the flag value is used.

## Config File

By default, Mantle looks for a file named `mantle.yaml` in the current working directory. You can specify a different path with the `--config` flag.

### Full Example

```yaml
database:
  url: postgres://mantle:mantle@localhost:5432/mantle?sslmode=disable

api:
  address: ":8080"

log:
  level: info

encryption:
  key: "your-64-char-hex-encoded-key-here"
```

### All Config File Fields

| Field | Type | Default | Description |
|---|---|---|---|
| `database.url` | string | `postgres://mantle:mantle@localhost:5432/mantle?sslmode=disable` | Postgres connection URL. |
| `api.address` | string | `:8080` | Listen address for `mantle serve`. Format: `host:port` or `:port`. Used by the HTTP API, webhook listener, and health endpoints. |
| `log.level` | string | `info` | Log verbosity. One of: `debug`, `info`, `warn`, `error`. |
| `encryption.key` | string | -- | Hex-encoded 32-byte master key for credential encryption. Required for `mantle secrets` commands. Generate with `openssl rand -hex 32`. |

### Config File Discovery

When you do not pass `--config`, Mantle searches for `mantle.yaml` in the current directory. If no config file is found, Mantle silently falls back to defaults. This is intentional -- most commands work fine with defaults when you use the provided `docker-compose.yml`.

When you pass `--config path/to/config.yaml` explicitly, Mantle requires that file to exist and be valid YAML. A missing or unparseable explicit config file is a hard error.

## Environment Variables

All environment variables use the `MANTLE_` prefix with underscores replacing dots and hyphens.

| Env Var | Config File Equivalent | Default |
|---|---|---|
| `MANTLE_DATABASE_URL` | `database.url` | `postgres://mantle:mantle@localhost:5432/mantle?sslmode=disable` |
| `MANTLE_API_ADDRESS` | `api.address` | `:8080` |
| `MANTLE_LOG_LEVEL` | `log.level` | `info` |
| `MANTLE_ENCRYPTION_KEY` | `encryption.key` | -- |

**Example:**

```bash
export MANTLE_DATABASE_URL="postgres://prod:secret@db.example.com:5432/mantle?sslmode=require"
export MANTLE_LOG_LEVEL="warn"
mantle init
```

Environment variables are useful in container deployments and CI pipelines where you do not want to mount a config file.

## CLI Flags

Every configuration option has a corresponding CLI flag. Flags take the highest priority.

| Flag | Config File Equivalent | Default |
|---|---|---|
| `--database-url` | `database.url` | `postgres://mantle:mantle@localhost:5432/mantle?sslmode=disable` |
| `--api-address` | `api.address` | `:8080` |
| `--log-level` | `log.level` | `info` |
| `--config` | -- | `mantle.yaml` (current directory) |

The `--config` flag has no environment variable or config file equivalent -- it controls which config file to load.

**Example:**

```bash
mantle init --database-url "postgres://prod:secret@db.example.com:5432/mantle?sslmode=require"
```

## Defaults

If you start Postgres using the included `docker-compose.yml` and run Mantle from the project directory, the defaults work without any configuration:

| Setting | Default Value |
|---|---|
| Database URL | `postgres://mantle:mantle@localhost:5432/mantle?sslmode=disable` |
| API address | `:8080` |
| Log level | `info` |

## Common Configurations

### Local Development

No config file needed. Start Postgres with `docker-compose up -d` and use defaults:

```bash
mantle init
mantle validate workflow.yaml
mantle apply workflow.yaml
```

### CI Pipeline

Use environment variables to avoid config files in CI:

```bash
export MANTLE_DATABASE_URL="postgres://ci:ci@localhost:5432/mantle_test?sslmode=disable"
mantle init
mantle validate workflow.yaml
mantle apply workflow.yaml
```

### Production

Use a `mantle.yaml` file with production values, or pass everything through environment variables:

```yaml
# mantle.yaml
database:
  url: postgres://mantle:${DB_PASSWORD}@db.internal:5432/mantle?sslmode=require

api:
  address: ":8080"

log:
  level: warn
```

Note: Mantle does not perform variable substitution in the config file. The `${DB_PASSWORD}` example above is illustrative -- use environment variables (`MANTLE_DATABASE_URL`) for secrets instead of embedding them in config files.

For production secrets management, set the encryption key through an environment variable rather than the config file:

```bash
export MANTLE_ENCRYPTION_KEY="$(openssl rand -hex 32)"
export MANTLE_DATABASE_URL="postgres://mantle:secret@db.internal:5432/mantle?sslmode=require"
mantle secrets create --name prod-openai --type openai --field api_key=sk-...
```

The encryption key has no default value. It is only required when you use `mantle secrets` commands or run workflows that reference credentials. All other Mantle commands work without it.

### Server Mode

When running `mantle serve`, the `api.address` setting controls which address the HTTP server, webhook listener, and health endpoints bind to:

```yaml
# mantle.yaml
database:
  url: postgres://mantle:secret@db.internal:5432/mantle?sslmode=require

api:
  address: ":8080"

log:
  level: info
```

Or with environment variables:

```bash
export MANTLE_DATABASE_URL="postgres://mantle:secret@db.internal:5432/mantle?sslmode=require"
export MANTLE_API_ADDRESS=":8080"
export MANTLE_ENCRYPTION_KEY="$(cat /run/secrets/mantle-key)"
mantle serve
```

The server runs migrations automatically on startup, so you do not need a separate `mantle init` step.

### Offline Commands

The `mantle validate` and `mantle version` commands do not require a database connection. They skip config loading entirely, so you can run them without Postgres, a config file, or any environment variables:

```bash
mantle validate workflow.yaml   # works anywhere, no database needed
mantle version                  # works anywhere
```

## Reference

See also:

- [CLI Reference](cli-reference.md) -- flag documentation for every command
- [Getting Started](getting-started.md) -- setup walkthrough using defaults
- [Secrets Guide](secrets-guide.md) -- credential encryption, creation, and key rotation
- [Server Guide](server-guide.md) -- running Mantle as a persistent server with triggers
