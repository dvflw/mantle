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

### Future Connectors

The connector system is designed for extensibility via gRPC plugins (HashiCorp go-plugin protocol). Custom connectors will run as subprocesses communicating over gRPC, keeping the core engine isolated from third-party code.

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

Mantle is a single Go binary that connects to a Postgres database. There are no other runtime dependencies.

```
+------------------+     +-----------+
|  mantle (binary)  |---->| Postgres  |
|                  |     |           |
|  - CLI commands  |     | - workflow_definitions
|  - Workflow engine|    | - workflow_executions
|  - Connectors    |     | - step_executions
|  - API server    |     | - credentials
|  - Cron scheduler|     +-----------+
|  - Webhook listener|
+------------------+
```

**Single binary.** No separate worker processes, message queues, or caches. The binary contains the CLI, the execution engine, the connectors, and the API server.

**Postgres for everything.** Workflow definitions, execution state, step checkpoints, and encrypted credentials all live in Postgres. This keeps the operational surface area small.

**Single-tenant in V1.** There is no authentication, authorization, or team scoping. Mantle assumes it is running in a trusted environment. Multi-tenancy and RBAC are planned for Phase 6.

## Further Reading

- [Getting Started](getting-started.md) -- install and run your first workflow
- [Workflow Reference](workflow-reference.md) -- complete YAML schema documentation
- [CLI Reference](cli-reference.md) -- every command and flag
- [Configuration](configuration.md) -- config file, env vars, and flag precedence
- [Secrets Guide](secrets-guide.md) -- credential encryption, creation, and key rotation
- [Server Guide](server-guide.md) -- running Mantle as a persistent server with triggers
