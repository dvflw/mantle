# Concepts

This page explains the core ideas behind Mantle's design: why workflows are treated as infrastructure, how versioning works, and the overall architecture.

## Infrastructure as Code Lifecycle

Mantle borrows the validate-plan-apply pattern from infrastructure tools like Terraform. Workflow definitions are not executed directly from YAML files. Instead, they go through a controlled deployment lifecycle:

```
mantle validate  -->  mantle plan  -->  mantle apply  -->  mantle run
   (offline)         (diff against       (store new        (execute
                      database)           version)          latest)
```

**validate** parses the YAML and checks it against structural rules: naming conventions, required fields, valid types, and correct durations. This runs offline with no database connection, so you can run it in a pre-commit hook or CI pipeline before anything touches the database.

**plan** compares your local file against the latest version stored in the database and shows a diff of what will change. Nothing is written.

**apply** validates the workflow, hashes the content with SHA-256, compares the hash against the latest stored version, and -- if the content changed -- inserts a new immutable version into the `workflow_definitions` table. If nothing changed, it reports "No changes" and does nothing.

**run** executes the latest applied version of a workflow, checkpointing each step to Postgres as it completes.

This lifecycle has a few important properties:

- **Every version is immutable.** Once applied, a version is never modified or deleted. Version 1 of a workflow always contains exactly what was applied as version 1.
- **Deployments are auditable.** You can trace what definition was active at any point in time by looking at version numbers and timestamps.
- **Validation is separated from storage.** You can validate dozens of files in CI without ever connecting to a database.

## Versioned Definitions

Every time you `mantle apply` a workflow with changed content, Mantle creates a new version with an incremented version number. The version history is strictly append-only.

```
mantle apply workflow.yaml   # Creates version 1
# edit workflow.yaml
mantle apply workflow.yaml   # Creates version 2
mantle apply workflow.yaml   # No changes — still version 2
# edit workflow.yaml again
mantle apply workflow.yaml   # Creates version 3
```

Mantle determines whether content has changed by comparing SHA-256 hashes of the raw YAML file content. If the hash matches the latest version, no new version is created. This means whitespace-only changes or comment changes do create new versions (since the raw bytes change), while applying the same file twice does not.

Each version record in the database stores:

| Column | Description |
|---|---|
| `id` | Unique UUID |
| `name` | Workflow name (e.g., `fetch-and-summarize`) |
| `version` | Integer version number, starting at 1 |
| `content` | The parsed workflow definition as JSON |
| `content_hash` | SHA-256 hash of the raw YAML file |
| `created_at` | Timestamp of when this version was applied |

## Connectors

Connectors are the integration points between Mantle and external systems. Each connector provides one or more actions that steps can invoke.

### Connector Summary

| Action | Description |
|---|---|
| `http/request` | HTTP requests to any URL |
| `ai/completion` | LLM chat completions (OpenAI-compatible) |
| `slack/send` | Send Slack messages |
| `slack/history` | Read Slack channel history |
| `postgres/query` | Execute SQL on external Postgres databases |
| `email/send` | Send email via SMTP |
| `s3/put` | Upload objects to S3 |
| `s3/get` | Download objects from S3 |
| `s3/list` | List objects in S3 |

Key design points:

- **HTTP Connector** -- general-purpose connector for REST APIs, webhooks, and any HTTP endpoint. JSON request/response bodies are automatically serialized and parsed.
- **AI Connector** -- BYOK (Bring Your Own Key) model. Supports structured output via `output_schema`, custom endpoints via `base_url`, and multi-turn tool use (function calling).
- **Slack Connector** -- `slack/send` and `slack/history` actions for sending messages and reading channel history.
- **Postgres Connector** -- executes parameterized SQL against external databases. Connects per-step and disconnects afterward.
- **Email Connector** -- sends email via SMTP with plaintext and HTML support.
- **S3 Connector** -- `s3/put`, `s3/get`, and `s3/list` for AWS S3 and S3-compatible services.

See the [Workflow Reference](/docs/workflow-reference/connectors) for complete parameter and output specifications.

## Plugin System

Plugins extend Mantle with third-party connector actions that run as subprocesses. This keeps the core engine isolated from external code while allowing the connector surface area to grow without modifying the Mantle binary.

A plugin is an executable binary that reads JSON from stdin and writes JSON to stdout. The engine invokes the plugin as a subprocess for each step execution.

```
Engine                     Plugin Process
  |                              |
  |-- spawn subprocess --------->|
  |-- write JSON to stdin ------>|
  |                              |-- execute action
  |<-- read JSON from stdout ----|
  |-- process terminates ------->|
```

Plugins are stored in the `.mantle/plugins/` directory. Use the CLI to manage them:

```bash
mantle plugins install ./path/to/my-plugin  # Copy binary to plugin directory
mantle plugins list                          # List installed plugins
mantle plugins remove my-plugin              # Remove a plugin
```

See the [Plugins Guide](/docs/plugins-guide) for a complete walkthrough of writing and testing a plugin.

## Shared Workflow Library

The workflow library lets teams publish reusable workflow templates and deploy them across environments and teams.

The library uses a two-step model:

1. **Publish** -- takes a workflow that has been `mantle apply`-ed and stores it as a shared template.
2. **Deploy** -- copies a shared template into a target team's workflow definitions as a new version.

```
Team A: mantle apply daily-report.yaml
        mantle library publish --workflow daily-report

Team B: mantle library list
        mantle library deploy --template daily-report
```

See the [CLI Reference](/docs/cli-reference/admin-commands) for command details.

## Observability

Mantle provides three observability mechanisms: Prometheus metrics, an immutable audit trail, and structured JSON logging.

### Prometheus Metrics

When running in server mode (`mantle serve`), Mantle exposes a `/metrics` endpoint in Prometheus exposition format.

| Metric | Type | Labels | Description |
|---|---|---|---|
| `mantle_workflow_executions_total` | Counter | `workflow`, `status` | Total workflow executions by name and outcome. |
| `mantle_step_executions_total` | Counter | `workflow`, `step`, `status` | Total step executions by workflow, step name, and outcome. |
| `mantle_step_duration_seconds` | Histogram | `workflow`, `step`, `action` | Step execution duration in seconds. |
| `mantle_connector_duration_seconds` | Histogram | `action` | Connector invocation duration in seconds. |
| `mantle_active_executions` | Gauge | -- | Number of currently running workflow executions. |

### Audit Trail

Every state-changing operation emits an immutable audit event to the `audit_events` table in Postgres. Query audit events with the `mantle audit` CLI command. See the [CLI Reference](/docs/cli-reference/admin-commands) for filter options.

### Structured JSON Logging

In server mode, Mantle emits structured JSON logs to stdout via Go's `slog` package. Configure the log level with `--log-level`, `MANTLE_LOG_LEVEL`, or `log.level` in `mantle.yaml`.

See the [Observability Guide](/docs/observability-guide) for detailed setup and example PromQL queries.

## Architecture Summary

Mantle is a single Go binary that connects to a Postgres database. Cloud secret stores are optional; plugins run as subprocesses.

```
+---------------------------+     +-----------+
|  mantle (binary)           |---->| Postgres  |
|                           |     |           |
|  - CLI commands           |     | - workflow_definitions
|  - Workflow engine        |     | - workflow_executions
|  - Built-in connectors   |     | - step_executions
|  - Plugin manager         |     | - credentials
|  - API server + /metrics  |     | - audit_events
|  - Cron scheduler         |     | - shared_workflows
|  - Webhook listener       |     +-----------+
|  - Audit emitter          |
|  - Secret resolver        |---->  Cloud Secret Stores
|                           |       (AWS, GCP, Azure — optional)
+---------------------------+
         |
         |--- spawn ---> Plugin subprocesses
                          (JSON stdin/stdout)
```

**Single binary.** No separate worker processes, message queues, or caches. The binary contains the CLI, the execution engine, the connectors, the plugin manager, and the API server.

**Postgres for everything.** Workflow definitions, execution state, step checkpoints, encrypted credentials, audit events, and shared templates all live in Postgres.

**Cloud secret stores are optional.** Mantle resolves credentials from Postgres first, then tries configured cloud backends (AWS Secrets Manager, GCP Secret Manager, Azure Key Vault), and finally falls back to environment variables.

**Plugins are isolated.** Third-party connectors run as subprocesses with a JSON stdin/stdout protocol. They cannot access the engine's memory or database directly.

## Further Reading

- [Execution Model](/docs/concepts/execution) -- checkpointing, parallel execution
- [CEL Expressions](/docs/concepts/expressions) -- data passing, conditional logic
- [Security Model](/docs/concepts/security) -- secrets, data residency
- [Getting Started](/docs/getting-started) -- install and run your first workflow
- [Workflow Reference](/docs/workflow-reference) -- complete YAML schema documentation
- [CLI Reference](/docs/cli-reference) -- every command and flag
- [Configuration](/docs/configuration) -- config file, env vars, and flag precedence
