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
version: 1

database:
  url: postgres://mantle:mantle@localhost:5432/mantle?sslmode=disable
  max_open_conns: 25
  max_idle_conns: 10
  conn_max_lifetime: 5m

api:
  address: ":8080"

log:
  level: info

encryption:
  key: "your-64-char-hex-encoded-key-here"

engine:
  worker_poll_interval: 200ms
  worker_max_backoff: 5s
  orchestrator_poll_interval: 500ms
  step_lease_duration: 60s
  orchestration_lease_duration: 120s
  ai_step_lease_duration: 300s
  reaper_interval: 30s
  step_output_max_bytes: 1048576
  default_max_tool_rounds: 10
  default_max_tool_calls_per_round: 10
  max_concurrent_executions_per_team: 10

storage:
  type: filesystem
  path: /var/lib/mantle/artifacts
  retention: "24h"

env:
  APP_NAME: "my-app"
  API_BASE_URL: "https://api.example.com"
```

### All Config File Fields

| Field | Type | Default | Description |
|---|---|---|---|
| `version` | integer | -- | Config file version. Must be `1` when present. Required for new config files starting in v0.4.0. |
| `database.url` | string | `postgres://mantle:mantle@localhost:5432/mantle?sslmode=disable` | Postgres connection URL. |
| `api.address` | string | `:8080` | Listen address for `mantle serve`. Format: `host:port` or `:port`. Used by the HTTP API, webhook listener, and health endpoints. |
| `log.level` | string | `info` | Log verbosity. One of: `debug`, `info`, `warn`, `error`. |
| `encryption.key` | string | -- | Hex-encoded 32-byte master key for credential encryption. Required for `mantle secrets` commands. Generate with `openssl rand -hex 32`. |
| `database.max_open_conns` | integer | `25` | Maximum number of open connections in the database pool. |
| `database.max_idle_conns` | integer | `10` | Maximum number of idle connections retained in the pool. |
| `database.conn_max_lifetime` | duration | `5m` | Maximum time a connection can be reused before it is closed. Uses Go duration format. |
| `engine.node_id` | string | `hostname:pid` | Unique identifier for this engine node. Auto-generated from hostname and PID if not set. |
| `engine.worker_poll_interval` | duration | `200ms` | How often workers poll for available step work. |
| `engine.worker_max_backoff` | duration | `5s` | Maximum backoff duration for worker polling when no work is available. |
| `engine.orchestrator_poll_interval` | duration | `500ms` | How often the orchestrator polls for workflow executions to advance. |
| `engine.step_lease_duration` | duration | `60s` | How long a worker holds a lease on a step before it can be reclaimed. |
| `engine.orchestration_lease_duration` | duration | `120s` | How long a node holds the orchestration lease for a workflow execution. |
| `engine.ai_step_lease_duration` | duration | `300s` | Lease duration for AI completion steps, which typically take longer than other steps. |
| `engine.reaper_interval` | duration | `30s` | How often the reaper checks for expired leases and stalled executions. |
| `engine.step_output_max_bytes` | integer | `1048576` | Maximum size in bytes for a single step's output. Outputs exceeding this limit are truncated. Default is 1 MB. |
| `engine.default_max_tool_rounds` | integer | `10` | Default maximum number of LLM-tool interaction rounds for AI steps with tools. Can be overridden per step with `max_tool_rounds`. |
| `engine.default_max_tool_calls_per_round` | integer | `10` | Default maximum number of tool calls the LLM can make per round. Can be overridden per step with `max_tool_calls_per_round`. |
| `engine.max_concurrent_executions_per_team` | integer | `0` | Maximum number of concurrent workflow executions allowed per team. When the limit is reached, new executions are queued. Set to `0` for unlimited. |
| `storage.type` | string | -- | Artifact storage backend. One of: `s3`, `filesystem`. Required if any workflow declares artifacts. |
| `storage.bucket` | string | -- | S3 bucket name. Required when `storage.type` is `s3`. |
| `storage.prefix` | string | -- | S3 key prefix for artifact storage. Optional. |
| `storage.path` | string | -- | Local directory path. Required when `storage.type` is `filesystem`. |
| `storage.retention` | duration | -- | How long to keep artifacts after workflow completion. Uses Go duration format (e.g., `24h`). Empty means no auto-cleanup. |
| `env.*` | map[string]string | -- | Key-value pairs available to CEL workflow expressions via `env.<KEY>`. See [Workflow Expression Variables](#workflow-expression-variables). |

:::caution[Deprecation: `tmp` renamed to `storage`]
The `tmp` configuration section has been renamed to `storage` in v0.4.0. The old `tmp` key still works but is deprecated and will be removed in a future release. Update your `mantle.yaml` to use `storage` instead.
:::

### Config File Discovery

When you do not pass `--config`, Mantle searches for `mantle.yaml` in the current directory. If no config file is found, Mantle silently falls back to defaults. This is intentional -- most commands work fine with defaults when you use the provided `docker-compose.yml`.

When you pass `--config path/to/config.yaml` explicitly, Mantle requires that file to exist and be valid YAML. A missing or unparseable explicit config file is a hard error.

## Workflow Expression Variables

The `env:` section in `mantle.yaml` defines key-value pairs that are available to CEL expressions in workflows via the `env` namespace. This is useful for environment-specific configuration that workflows need at runtime (API base URLs, feature flags, region names, etc.) without hardcoding values in workflow definitions.

### Syntax

```yaml
# mantle.yaml
env:
  APP_NAME: "my-app"
  API_BASE_URL: "https://api.example.com"
  REGION: "us-east-1"
  DEBUG: "true"
```

### Usage in Workflows

Values defined in the `env:` section are accessible in any CEL expression via `env.<KEY>`:

```yaml
# workflow.yaml
name: deploy
steps:
  - name: notify
    connector: http/request
    params:
      url: "{{ env.API_BASE_URL }}/deployments"
      body: '{"app": "{{ env.APP_NAME }}", "region": "{{ env.REGION }}"}'
```

### Precedence

The `env:` config values merge with `MANTLE_ENV_*` OS environment variables. When the same key exists in both sources, the OS environment variable wins:

1. **`MANTLE_ENV_*` environment variables** (highest priority) -- stripped of the `MANTLE_ENV_` prefix
2. **`env:` section in `mantle.yaml`** (lower priority)

When an override occurs, Mantle logs an info message:

```text
INFO env variable overrides config key=MANTLE_ENV_REGION config_key=env.REGION
```

This allows operators to override config-file defaults without editing YAML, which is useful in CI pipelines and container deployments.

**Example override:**

```yaml
# mantle.yaml
env:
  REGION: "us-east-1"
```

```bash
# Override REGION for this deployment
export MANTLE_ENV_REGION="eu-west-1"
mantle run deploy  # env.REGION evaluates to "eu-west-1"
```

## Environment Variables

All environment variables use the `MANTLE_` prefix with underscores replacing dots and hyphens.

| Env Var | Config File Equivalent | Default |
|---|---|---|
| `MANTLE_DATABASE_URL` | `database.url` | `postgres://mantle:mantle@localhost:5432/mantle?sslmode=disable` |
| `MANTLE_API_ADDRESS` | `api.address` | `:8080` |
| `MANTLE_LOG_LEVEL` | `log.level` | `info` |
| `MANTLE_ENCRYPTION_KEY` | `encryption.key` | -- |
| `MANTLE_DATABASE_MAX_OPEN_CONNS` | `database.max_open_conns` | `25` |
| `MANTLE_DATABASE_MAX_IDLE_CONNS` | `database.max_idle_conns` | `10` |
| `MANTLE_DATABASE_CONN_MAX_LIFETIME` | `database.conn_max_lifetime` | `5m` |
| `MANTLE_ENGINE_NODE_ID` | `engine.node_id` | `hostname:pid` |
| `MANTLE_ENGINE_WORKER_POLL_INTERVAL` | `engine.worker_poll_interval` | `200ms` |
| `MANTLE_ENGINE_WORKER_MAX_BACKOFF` | `engine.worker_max_backoff` | `5s` |
| `MANTLE_ENGINE_ORCHESTRATOR_POLL_INTERVAL` | `engine.orchestrator_poll_interval` | `500ms` |
| `MANTLE_ENGINE_STEP_LEASE_DURATION` | `engine.step_lease_duration` | `60s` |
| `MANTLE_ENGINE_ORCHESTRATION_LEASE_DURATION` | `engine.orchestration_lease_duration` | `120s` |
| `MANTLE_ENGINE_AI_STEP_LEASE_DURATION` | `engine.ai_step_lease_duration` | `300s` |
| `MANTLE_ENGINE_REAPER_INTERVAL` | `engine.reaper_interval` | `30s` |
| `MANTLE_ENGINE_STEP_OUTPUT_MAX_BYTES` | `engine.step_output_max_bytes` | `1048576` |
| `MANTLE_ENGINE_DEFAULT_MAX_TOOL_ROUNDS` | `engine.default_max_tool_rounds` | `10` |
| `MANTLE_ENGINE_DEFAULT_MAX_TOOL_CALLS_PER_ROUND` | `engine.default_max_tool_calls_per_round` | `10` |
| `MANTLE_ENGINE_MAX_CONCURRENT_EXECUTIONS_PER_TEAM` | `engine.max_concurrent_executions_per_team` | `10` |
| `MANTLE_STORAGE_TYPE` | `storage.type` | -- |
| `MANTLE_STORAGE_BUCKET` | `storage.bucket` | -- |
| `MANTLE_STORAGE_PREFIX` | `storage.prefix` | -- |
| `MANTLE_STORAGE_PATH` | `storage.path` | -- |
| `MANTLE_STORAGE_RETENTION` | `storage.retention` | -- |

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
version: 1

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
version: 1

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

## Cloud Secret Backend Configuration

Mantle can resolve credentials from external cloud secret stores. Cloud backends are configured through environment variables -- there are no config file fields for these.

| Env Var | Description |
|---|---|
| `AWS_REGION` or `AWS_DEFAULT_REGION` | AWS region for Secrets Manager. Enables the AWS backend when AWS credentials are available. |
| `MANTLE_GCP_PROJECT` | GCP project ID for Secret Manager. Enables the GCP backend. |
| `MANTLE_AZURE_VAULT_URL` | Azure Key Vault URL (e.g., `https://my-vault.vault.azure.net/`). Enables the Azure backend. |

Cloud backends are optional. If none are configured, Mantle resolves credentials from the Postgres store and environment variable fallback only.

Each cloud backend uses its respective SDK's default credential chain:

- **AWS**: environment variables, shared credentials file, EC2/ECS instance profile
- **GCP**: Application Default Credentials (`gcloud auth application-default login`, Workload Identity, service account key)
- **Azure**: DefaultAzureCredential (`az login`, managed identity, environment variables)

See the [Secrets Guide](secrets-guide.md#cloud-secret-backends) for detailed setup instructions and IAM requirements.

## Plugin Configuration

Plugins are stored in `.mantle/plugins/` relative to the current working directory. This location is not currently configurable via the config file -- it uses a fixed path.

## Reference

See also:

- [CLI Reference](/docs/cli-reference) -- flag documentation for every command
- [Getting Started](/docs/getting-started) -- setup walkthrough using defaults
- [Secrets Guide](/docs/secrets-guide) -- credential encryption, cloud backends, and key rotation
- [Server Guide](/docs/server-guide) -- running Mantle as a persistent server with triggers
- [Plugins Guide](/docs/plugins-guide) -- writing and managing third-party connector plugins
- [Observability Guide](/docs/observability-guide) -- Prometheus metrics, audit trail, and structured logging
