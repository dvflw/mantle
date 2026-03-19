# Concepts

This page explains the core ideas behind Mantle's design: why workflows are treated as infrastructure, how versioning works, what checkpointing guarantees (and does not guarantee), and how data flows between steps.

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

## Checkpoint and Resume

When Mantle executes a workflow, each step's result is checkpointed to Postgres before the next step begins. If the process crashes mid-execution, it resumes from the last completed step rather than starting over.

### What Checkpointing Guarantees

- **No duplicate internal work.** A step that completed and was checkpointed before a crash is not re-executed after recovery.
- **Durable state.** Step outputs survive process restarts because they are stored in Postgres, not in memory.
- **Crash recovery.** A workflow execution can be resumed by any Mantle instance with access to the same database.

### What Checkpointing Does Not Guarantee

- **Exactly-once external side effects.** If a step makes an HTTP POST and the process crashes after the POST completes but before the checkpoint is written, the POST will be repeated on recovery. This is inherent to any system that interacts with external services. Use idempotency keys in your external APIs to handle this.
- **Atomicity across steps.** Each step is independent. There is no rollback of previously completed steps if a later step fails.

The database schema for execution tracking uses three tables:

- `workflow_executions` -- one row per workflow run, tracking overall status (`pending`, `running`, `completed`, `failed`, `cancelled`)
- `step_executions` -- one row per step attempt, tracking status, output, and errors
- Each step attempt is uniquely identified by `(execution_id, step_name, attempt)`, supporting retries

## CEL Expressions and Data Flow

Data flows between steps through [CEL (Common Expression Language)](https://github.com/google/cel-go) expressions. CEL is a small, fast, non-Turing-complete expression language designed by Google for security and policy evaluation.

### Three Namespaces

| Namespace | Example | Description |
|---|---|---|
| `inputs` | `inputs.url` | Values passed when the workflow is triggered. |
| `steps` | `steps.fetch-data.output.body` | Output from a previously completed step. |
| `env` | `env.API_TOKEN` | Environment variables available to the engine. |

### Where CEL Appears

**Conditional execution** -- the `if` field on a step:

```yaml
if: "steps.fetch-data.output.status_code == 200"
```

The step runs only when this expression evaluates to `true`. If the expression evaluates to `false`, the step is skipped.

**Template interpolation** -- double-brace syntax in `params`:

```yaml
params:
  url: "{{ inputs.url }}"
  prompt: "Summarize: {{ steps.fetch-data.output.body }}"
```

Template expressions are evaluated and their results are substituted into the string.

### Data Flow Example

Consider this workflow:

```yaml
inputs:
  url:
    type: string

steps:
  - name: fetch-data
    action: http/request
    params:
      method: GET
      url: "{{ inputs.url }}"

  - name: summarize
    action: ai/completion
    params:
      provider: openai
      model: gpt-4o
      prompt: "Summarize: {{ steps.fetch-data.output.body }}"
```

The data flows like this:

1. The caller provides `url` as an input when triggering the workflow.
2. Step `fetch-data` reads `inputs.url` and makes an HTTP GET request.
3. The HTTP connector returns output with fields like `status_code`, `headers`, and `body`.
4. Step `summarize` reads `steps.fetch-data.output.body` to build its prompt.
5. The AI connector returns the completion result.

Each step can only reference outputs from steps that appear earlier in the list. Referencing a step that has not yet executed is an error.

## Connectors

Connectors are the integration points between Mantle and external systems. Each connector provides one or more actions that steps can invoke.

### HTTP Connector

The `http/request` action makes HTTP requests. It is the general-purpose connector for interacting with REST APIs, webhooks, and any HTTP endpoint.

Key design points:

- JSON request bodies are automatically serialized
- JSON response bodies are automatically parsed into structured data accessible via CEL
- You control headers, method, URL, and body through step params

See the [Workflow Reference](workflow-reference.md#httprequest) for the complete parameter and output specification.

### AI Connector

The `ai/completion` action sends prompts to OpenAI-compatible chat completion APIs.

Key design points:

- **BYOK (Bring Your Own Key)** -- Mantle does not proxy through a hosted service. You provide your own API keys through the secrets system and reference them with the `credential` field on your workflow step.
- **Structured output** -- you can pass an `output_schema` parameter with a JSON Schema, and the model returns JSON conforming to that schema. The parsed result is available as `steps.STEP_NAME.output.json`.
- **Custom endpoints** -- the `base_url` parameter lets you point to any OpenAI-compatible API (Azure OpenAI, Ollama, vLLM, etc.) instead of the default `https://api.openai.com/v1`.
- **No tool use in V1** -- function calling and tool use are planned for V1.1.

See the [Workflow Reference](workflow-reference.md#aicompletion) for the complete parameter and output specification.

### Slack Connector

The `slack/send` and `slack/history` actions interact with the Slack Web API. They handle authentication, request formatting, and error parsing for you.

Use cases:
- Sending notifications to a team channel when a workflow succeeds or fails
- Reading recent messages from a channel to summarize or process

See the [Workflow Reference](workflow-reference.md#slacksend) for parameters and output.

### Postgres Connector

The `postgres/query` action executes parameterized SQL against external Postgres databases. It connects per-step and disconnects afterward, keeping the connector stateless.

Use cases:
- Reading data from a reporting database to feed into an AI summarization step
- Writing workflow results back to a business database
- Running scheduled data cleanup queries

See the [Workflow Reference](workflow-reference.md#postgresquery) for parameters and output.

### Email Connector

The `email/send` action sends emails via SMTP. It supports plaintext and HTML content, multiple recipients, and configurable SMTP servers.

See the [Workflow Reference](workflow-reference.md#emailsend) for parameters and output.

### S3 Connector

The `s3/put`, `s3/get`, and `s3/list` actions interact with AWS S3 and S3-compatible storage services (MinIO, DigitalOcean Spaces, Backblaze B2). The `endpoint` credential field allows you to point to any S3-compatible API.

See the [Workflow Reference](workflow-reference.md#s3put) for parameters and output.

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

## Plugin System

Plugins extend Mantle with third-party connector actions that run as subprocesses. This keeps the core engine isolated from external code while allowing the connector surface area to grow without modifying the Mantle binary.

### How Plugins Work

A plugin is an executable binary that reads JSON from stdin and writes JSON to stdout. The engine invokes the plugin as a subprocess for each step execution, passing the action name, parameters, and credential fields as a JSON payload.

```
Engine                     Plugin Process
  |                              |
  |-- spawn subprocess --------->|
  |-- write JSON to stdin ------>|
  |                              |-- execute action
  |<-- read JSON from stdout ----|
  |-- process terminates ------->|
```

### Plugin Contract

The JSON input format:

```json
{
  "action": "my-plugin/do-thing",
  "params": {"key": "value"},
  "credential": {"api_key": "secret"}
}
```

The JSON output format:

```json
{
  "result": "success",
  "data": {"processed": true}
}
```

If the plugin writes to stderr or exits with a non-zero code, the step fails with the stderr content as the error message.

### Protobuf Definition

The plugin contract is formally defined in `proto/connector.proto`. While the current V1.1 implementation uses the simpler JSON stdin/stdout protocol, the protobuf definition serves as the specification for a future gRPC-based plugin protocol.

The service defines three RPCs:

- **Execute** -- runs the connector action with parameters and credentials
- **Validate** -- checks whether parameters are valid for this connector
- **Describe** -- returns metadata about the connector's supported actions

### Plugin Management

Plugins are stored in the `.mantle/plugins/` directory. Use the CLI to manage them:

```bash
mantle plugins install ./path/to/my-plugin  # Copy binary to plugin directory
mantle plugins list                          # List installed plugins
mantle plugins remove my-plugin              # Remove a plugin
```

See the [Plugins Guide](plugins-guide.md) for a complete walkthrough of writing and testing a plugin.

## Shared Workflow Library

The workflow library lets teams publish reusable workflow templates and deploy them across environments and teams. This is Mantle's mechanism for sharing best-practice workflows without copy-pasting YAML files.

### Publish/Deploy Model

The library uses a two-step model:

1. **Publish** -- takes a workflow that has been `mantle apply`-ed and stores it as a shared template. The template includes the workflow's name, description, and full definition.

2. **Deploy** -- copies a shared template into a target team's workflow definitions as a new version. The deployed workflow behaves identically to one created through `mantle apply`.

```
Team A: mantle apply daily-report.yaml
        mantle library publish --workflow daily-report

Team B: mantle library list
        mantle library deploy --template daily-report
```

Publishing the same name again updates the template. Deploying the same template again creates a new version, not a duplicate.

### When to Use the Library

- Sharing standard operational workflows (health checks, data syncs) across teams
- Creating starter templates for common patterns (fetch-transform-notify)
- Distributing approved workflows in a multi-tenant environment

See the [CLI Reference](cli-reference.md#mantle-library) for command details.

## Observability

Mantle provides three observability mechanisms: Prometheus metrics, an immutable audit trail, and structured JSON logging. Together, they give you visibility into what your workflows are doing, how they are performing, and who changed what.

### Prometheus Metrics

When running in server mode (`mantle serve`), Mantle exposes a `/metrics` endpoint in Prometheus exposition format. Scrape this endpoint with Prometheus, Grafana Agent, or any compatible collector.

**Exposed metrics:**

| Metric | Type | Labels | Description |
|---|---|---|---|
| `mantle_workflow_executions_total` | Counter | `workflow`, `status` | Total workflow executions by name and outcome. |
| `mantle_step_executions_total` | Counter | `workflow`, `step`, `status` | Total step executions by workflow, step name, and outcome. |
| `mantle_step_duration_seconds` | Histogram | `workflow`, `step`, `action` | Step execution duration in seconds. |
| `mantle_connector_duration_seconds` | Histogram | `action` | Connector invocation duration in seconds. |
| `mantle_active_executions` | Gauge | -- | Number of currently running workflow executions. |

### Audit Trail

Every state-changing operation emits an immutable audit event to the `audit_events` table in Postgres. Events are append-only -- they cannot be modified or deleted.

Query audit events with the `mantle audit` CLI command. See the [CLI Reference](cli-reference.md#mantle-audit) for filter options.

### Structured JSON Logging

In server mode, Mantle emits structured JSON logs to stdout via Go's `slog` package. Each log line is a JSON object with `time`, `level`, `msg`, and contextual fields.

```json
{"time":"2026-03-18T14:30:00.000Z","level":"INFO","msg":"server listening","address":":8080"}
{"time":"2026-03-18T14:30:01.000Z","level":"INFO","msg":"cron scheduler started"}
{"time":"2026-03-18T14:30:05.000Z","level":"INFO","msg":"workflow execution completed","workflow":"hello-world","execution_id":"abc123"}
```

Configure the log level with the `--log-level` flag, `MANTLE_LOG_LEVEL` environment variable, or `log.level` in `mantle.yaml`. Levels: `debug`, `info`, `warn`, `error`.

The JSON format integrates directly with log aggregation systems like the ELK stack, Datadog, Grafana Loki, and any tool that ingests structured JSON.

## Secrets and Credential Resolution

Mantle treats secrets (API keys, tokens, credentials) as opaque handles that are resolved at connector invocation time. You never put raw secret values in workflow YAML. Instead, you create a named credential with `mantle secrets create` and reference it by name in your workflow step's `credential` field.

### Credential Types

Each credential has a type that defines its schema:

| Type | Fields | Use Case |
|---|---|---|
| `generic` | `key` (required) | General-purpose API key |
| `bearer` | `token` (required) | Bearer token authentication |
| `openai` | `api_key` (required), `org_id` (optional) | OpenAI API access |
| `basic` | `username` (required), `password` (required) | HTTP Basic authentication |

Types enforce that the right fields are present when you create a credential, reducing misconfiguration errors at runtime.

### How Credential Resolution Works

When the engine reaches a step with a `credential` field, it resolves the credential name before invoking the connector:

1. **Postgres lookup** -- the engine queries the credentials table, decrypts the stored fields using AES-256-GCM, and passes them to the connector.
2. **Environment variable fallback** -- if the credential is not found in Postgres, the engine checks for an environment variable named `MANTLE_SECRET_<UPPER_NAME>` (hyphens are replaced with underscores). The env var value is returned as a single `key` field, equivalent to a `generic` credential.

The resolved credential fields are injected directly into the connector as an internal `_credential` parameter. They are never visible in CEL expressions, step outputs, or execution logs.

### Security Properties

- **Encrypted at rest** -- credential field values are encrypted with AES-256-GCM before being written to Postgres. The encryption key is not stored in the database.
- **Never in expressions** -- you cannot reference `credential` data in CEL templates or `if` conditions. The credential is resolved inside the connector, not in the expression engine.
- **Never in logs** -- credential values do not appear in execution logs, step outputs, or error messages.
- **Typed validation** -- creating a credential validates that all required fields for the type are present.

For the full operational guide, see the [Secrets Guide](secrets-guide.md).

## Triggers and Server Mode

Up to this point, every concept on this page describes workflows that are triggered manually: you run `mantle run` and the engine executes the latest applied version. Triggers and server mode introduce automatic execution.

### Server Mode

`mantle serve` starts Mantle as a long-running process. Instead of executing a single workflow and exiting, the server stays up and:

- Accepts HTTP API requests to trigger and cancel executions
- Polls for due cron triggers every 30 seconds
- Listens for incoming webhook requests
- Serves health endpoints for load balancer and Kubernetes probes

The server runs migrations automatically on startup, so you do not need a separate `mantle init` step in your deployment pipeline.

### Cron Triggers

A cron trigger tells Mantle to start a new workflow execution on a recurring schedule. The schedule uses standard 5-field cron syntax (minute, hour, day-of-month, month, day-of-week).

```yaml
triggers:
  - type: cron
    schedule: "*/5 * * * *"
```

The cron scheduler is built into the `mantle serve` process. It polls every 30 seconds, checks which cron triggers are due, and starts new executions for them. Cron triggers have no effect when running workflows manually with `mantle run` -- they only fire in server mode.

### Webhook Triggers

A webhook trigger tells Mantle to start a new workflow execution when an HTTP POST arrives at a specific path. The request body is parsed as JSON and made available as `trigger.payload` in CEL expressions.

```yaml
triggers:
  - type: webhook
    path: "/hooks/on-deploy"
```

This is the primary way to integrate Mantle with external systems: CI pipelines, monitoring alerts, GitHub webhooks, and third-party SaaS tools can all POST to a webhook endpoint to kick off a workflow.

### Trigger Lifecycle

Triggers are managed through the same IaC lifecycle as the rest of the workflow definition. When you run `mantle apply`:

- Triggers defined in the YAML are registered (or updated) in the database
- Triggers that were previously registered but are no longer in the YAML are deregistered

This means the workflow YAML file is the single source of truth for trigger configuration. You do not create, update, or delete triggers separately -- they are part of the apply cycle.

```
# First apply: registers the cron trigger
mantle apply workflow.yaml

# Edit workflow.yaml: change schedule from */5 to */10
mantle apply workflow.yaml    # Updates the trigger

# Edit workflow.yaml: remove the triggers section entirely
mantle apply workflow.yaml    # Deregisters all triggers
```

### Cron vs Webhook: When to Use Which

| Use Case | Trigger Type | Why |
|---|---|---|
| Periodic data sync | Cron | Runs on a fixed schedule regardless of external events |
| Deploy notifications | Webhook | Fires in response to an external event (CI pipeline) |
| Daily report generation | Cron | Time-based, no external signal needed |
| GitHub push handler | Webhook | Event-driven, triggered by an external system |
| Scheduled cleanup | Cron | Maintenance task on a recurring schedule |

Many workflows benefit from both: a cron trigger for periodic runs and a webhook trigger for on-demand execution by external systems.

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

**Postgres for everything.** Workflow definitions, execution state, step checkpoints, encrypted credentials, audit events, and shared templates all live in Postgres. This keeps the operational surface area small.

**Cloud secret stores are optional.** Mantle resolves credentials from Postgres first, then tries configured cloud backends (AWS Secrets Manager, GCP Secret Manager, Azure Key Vault), and finally falls back to environment variables.

**Plugins are isolated.** Third-party connectors run as subprocesses with a JSON stdin/stdout protocol. They cannot access the engine's memory or database directly.

## Further Reading

- [Getting Started](getting-started.md) -- install and run your first workflow
- [Workflow Reference](workflow-reference.md) -- complete YAML schema documentation
- [CLI Reference](cli-reference.md) -- every command and flag
- [Configuration](configuration.md) -- config file, env vars, and flag precedence
- [Secrets Guide](secrets-guide.md) -- credential encryption, cloud backends, and key rotation
- [Server Guide](server-guide.md) -- running Mantle as a persistent server with triggers
- [Plugins Guide](plugins-guide.md) -- writing and managing third-party connector plugins
- [Observability Guide](observability-guide.md) -- Prometheus metrics, audit trail, and structured logging
