# Getting Started

After completing this guide, you will have Mantle installed, a Postgres database running, and your first workflow executing -- all in under five minutes. From there, the guide progressively introduces data passing, conditional logic, AI/LLM integration, server mode, and multi-tenancy.

## What is Mantle?

Mantle is a headless AI workflow automation platform. You define workflows as YAML, deploy them through an infrastructure-as-code lifecycle (validate, plan, apply), and execute them against a Postgres-backed engine. It ships as a single Go binary -- bring your own API keys, bring your own database, no hosted runtime required.

## Prerequisites

You need the following installed on your machine:

- **Go 1.25+** -- [install instructions](https://go.dev/doc/install)
- **Docker and Docker Compose** -- [install instructions](https://docs.docker.com/get-docker/)
- **Make** -- included on macOS and most Linux distributions

Verify your setup:

```bash
go version    # go1.25 or later
docker --version
```

## Install and Start (< 2 minutes)

Clone the repository, start Postgres, build the binary, and run migrations:

```bash
git clone https://github.com/dvflw/mantle.git && cd mantle
docker compose up -d
make build
./mantle init
```

The `docker compose up -d` command starts Postgres 16 on `localhost:5432` with user `mantle`, password `mantle`, and database `mantle`. The `make build` command produces a single `mantle` binary in the project root. The `mantle init` command creates all required database tables.

The default database URL uses `sslmode=disable`, which is correct for local development with the provided Docker Compose setup. For production, always use `sslmode=require` or `sslmode=verify-full`:

```bash
export MANTLE_DATABASE_URL="postgres://mantle:secret@db.example.com:5432/mantle?sslmode=require"
```

See [Configuration](configuration.md) for all database options.

You should see:

```
Running migrations...
Migrations complete.
```

Optionally, move the binary onto your PATH:

```bash
sudo mv mantle /usr/local/bin/
```

Verify it works:

```bash
mantle version
# mantle v0.1.0 (791fa83, built 2026-03-18T00:00:00Z)
```

## Your First Workflow (< 3 minutes)

The `examples/` directory includes several ready-to-run workflows. Start with the simplest one -- a single HTTP GET request.

Look at `examples/hello-world.yaml`:

```yaml
name: hello-world
description: Fetch a random fact from a public API — the simplest possible Mantle workflow

steps:
  - name: fetch
    action: http/request
    params:
      method: GET
      url: "https://jsonplaceholder.typicode.com/posts/1"
```

This workflow has one step: it sends a GET request to the JSONPlaceholder API and returns the response.

### Step 1: Validate

Check the workflow for structural errors. This runs offline -- no database connection required:

```bash
mantle validate examples/hello-world.yaml
```

Output:

```
hello-world.yaml: valid
```

If there are errors, Mantle reports them with file, line, and column numbers:

```
bad-workflow.yaml:1:1: error: name must match ^[a-z][a-z0-9-]*$ (name)
```

### Step 2: Apply

Store the workflow definition as a new immutable version in the database:

```bash
mantle apply examples/hello-world.yaml
```

Output:

```
Applied hello-world version 1
```

Every time you edit a workflow and re-apply, Mantle creates a new version. If the content has not changed, it tells you:

```
No changes — hello-world is already at version 1
```

You can also preview what will change before applying:

```bash
mantle plan examples/hello-world.yaml
```

```
No changes — hello-world is at version 1
```

### Step 3: Run

Execute the workflow by name:

```bash
mantle run hello-world
```

Output:

```
Running hello-world (version 1)...
Execution a1b2c3d4-e5f6-7890-abcd-ef1234567890: completed
  fetch: completed
```

### Step 4: View Logs

Inspect the execution with the execution ID from the previous step:

```bash
mantle logs a1b2c3d4-e5f6-7890-abcd-ef1234567890
```

Output:

```
Execution: a1b2c3d4-e5f6-7890-abcd-ef1234567890
Workflow:  hello-world (version 1)
Status:    completed
Started:   2026-03-18T14:30:00Z
Completed: 2026-03-18T14:30:01Z
Duration:  1.042s

Steps:
  fetch           completed (1.0s)
```

If a step fails, the error appears below the step name:

```
Steps:
  fetch           failed (0.5s)
    error: http/request: GET https://jsonplaceholder.typicode.com/posts/1: connection refused
```

You can also get a quick status summary with `mantle status <execution-id>`.

## Data Passing Between Steps

Workflows become powerful when steps pass data to each other. Look at `examples/chained-requests.yaml`:

```yaml
name: chained-requests
description: >
  Fetch a user from a public API, then fetch their posts using the user's ID.
  Demonstrates CEL data passing between steps via steps.<name>.output.

steps:
  - name: get-user
    action: http/request
    params:
      method: GET
      url: "https://jsonplaceholder.typicode.com/users/1"

  - name: get-user-posts
    action: http/request
    params:
      method: GET
      url: "https://jsonplaceholder.typicode.com/posts?userId={{ steps['get-user'].output.json.id }}"
```

The key line is the second step's URL. The expression `{{ steps['get-user'].output.json.id }}` reads the JSON response from the `get-user` step and extracts the `id` field.

Apply and run it:

```bash
mantle apply examples/chained-requests.yaml
mantle run chained-requests
```

```
Running chained-requests (version 1)...
Execution b2c3d4e5-f6a7-8901-bcde-f12345678901: completed
  get-user: completed
  get-user-posts: completed
```

### CEL Expression Syntax

Mantle uses [CEL (Common Expression Language)](https://github.com/google/cel-go) for data passing and conditional logic. The essentials:

- **Access step output:** `steps['step-name'].output.json.field`
- **Access inputs:** `inputs.field_name`
- **Bracket notation is required** when step names contain hyphens: `steps['get-user']` (not `steps.get-user`)
- **Dot notation works** for step names without hyphens: `steps.summarize.output.json.summary`
- **Template strings** use `{{ }}` delimiters inside `params` values

## Conditional Execution

Steps can run conditionally based on the output of previous steps. Look at `examples/conditional-workflow.yaml`:

```yaml
name: conditional-workflow
description: >
  Fetch todos for a user, then conditionally post a summary only if there are
  incomplete todos. Demonstrates conditional execution with if: and retry policies.

inputs:
  user_id:
    type: string
    description: JSONPlaceholder user ID (1-10)

steps:
  - name: get-todos
    action: http/request
    timeout: "10s"
    retry:
      max_attempts: 3
      backoff: exponential
    params:
      method: GET
      url: "https://jsonplaceholder.typicode.com/todos?userId={{ inputs.user_id }}"

  - name: post-summary
    action: http/request
    if: "steps['get-todos'].output.status == 200"
    params:
      method: POST
      url: "https://jsonplaceholder.typicode.com/posts"
      headers:
        Content-Type: "application/json"
      body:
        title: "Todo summary"
        body: "Fetched todos for user {{ inputs.user_id }}"
        userId: "{{ inputs.user_id }}"
```

This workflow introduces three features:

- **`inputs`** -- the workflow declares a `user_id` input, passed at runtime with `--input`
- **`if`** -- the `post-summary` step only runs when the CEL expression evaluates to true
- **`retry` and `timeout`** -- the `get-todos` step retries up to 3 times with exponential backoff and times out after 10 seconds

Apply and run it:

```bash
mantle apply examples/conditional-workflow.yaml
mantle run conditional-workflow --input user_id=3
```

```
Running conditional-workflow (version 1)...
Execution c3d4e5f6-a7b8-9012-cdef-123456789012: completed
  get-todos: completed
  post-summary: completed
```

You can pass multiple inputs by repeating the `--input` flag:

```bash
mantle run my-workflow --input key1=value1 --input key2=value2
```

## Using AI/LLM

Mantle includes a built-in AI connector that supports OpenAI-compatible APIs. Before using it, you need to store your API key as an encrypted credential.

### Set Up Credentials

Generate an encryption key and export it:

```bash
export MANTLE_ENCRYPTION_KEY=$(openssl rand -hex 32)
```

Store your OpenAI API key:

```bash
mantle secrets create --name openai --type openai --field api_key=sk-proj-your-key-here
```

```
Created credential "openai" (type: openai)
```

The credential is encrypted at rest with AES-256-GCM. The raw API key is never stored in plaintext, never exposed in logs, and never available in CEL expressions. See the [Secrets Guide](secrets-guide.md) for credential types and the full security model.

### AI Completion Step

Here is a workflow that fetches a webpage and uses an LLM to extract structured data (from `examples/ai-structured-extraction.yaml`):

```yaml
name: ai-structured-extraction
description: >
  Fetch a webpage and use an LLM with output_schema to extract structured data
  (title, author, key topics). Demonstrates enforcing JSON structure from AI output.

inputs:
  url:
    type: string
    description: URL of the page to fetch and extract data from

steps:
  - name: fetch-page
    action: http/request
    timeout: "15s"
    retry:
      max_attempts: 2
      backoff: exponential
    params:
      method: GET
      url: "{{ inputs.url }}"

  - name: extract-metadata
    action: ai/completion
    credential: openai
    params:
      model: gpt-4o
      system_prompt: >
        You are a structured data extraction engine. Given raw page content,
        extract the requested fields accurately. If a field cannot be determined,
        use null or an empty value as appropriate.
      prompt: >
        Extract the following metadata from this page content:

        {{ steps['fetch-page'].output.body }}
      output_schema:
        type: object
        properties:
          title:
            type: string
          author:
            type: string
          key_topics:
            type: array
            items:
              type: string
        required:
          - title
          - author
          - key_topics
        additionalProperties: false
```

The `credential: openai` field tells the engine to resolve the `openai` credential you created earlier. The `output_schema` field enforces structured JSON output from the model -- the response is guaranteed to match the schema.

Apply and run it:

```bash
mantle apply examples/ai-structured-extraction.yaml
mantle run ai-structured-extraction --input url=https://example.com
```

```
Running ai-structured-extraction (version 1)...
Execution d4e5f6a7-b8c9-0123-defa-234567890123: completed
  fetch-page: completed
  extract-metadata: completed
```

### Key AI Connector Details

| Field | Description |
|---|---|
| `action` | `ai/completion` for chat completions |
| `credential` | Name of a stored credential (type `openai`) |
| `model` | Model name (e.g., `gpt-4o`, `gpt-4o-mini`) |
| `prompt` | User message -- supports CEL template expressions |
| `system_prompt` | Optional system message to set model behavior |
| `output_schema` | Optional JSON Schema -- enforces structured output |

## Server Mode and Triggers

So far you have been running workflows manually with `mantle run`. In production, you start Mantle as a persistent server that supports cron schedules and webhook triggers.

### Define Triggers

Add a `triggers` section to your workflow YAML:

```yaml
name: api-health-check
description: Check API health hourly and on demand

triggers:
  - type: cron
    schedule: "0 * * * *"
  - type: webhook
    path: "/hooks/api-health-check"

steps:
  - name: check-api
    action: http/request
    timeout: "10s"
    retry:
      max_attempts: 3
      backoff: exponential
    params:
      method: GET
      url: https://api.example.com/health

  - name: alert-on-failure
    action: http/request
    if: "steps['check-api'].output.status != 200"
    params:
      method: POST
      url: https://hooks.slack.com/services/T00/B00/xxx
      body:
        text: "API health check failed with status {{ steps['check-api'].output.status }}"
```

Apply the workflow, then start the server:

```bash
mantle apply api-health-check.yaml
mantle serve
```

```
Running migrations...
Migrations complete.
Starting server on :8080
Cron scheduler started (poll interval: 30s)
```

The server runs migrations on startup, starts the HTTP API on `:8080`, and polls for due cron triggers every 30 seconds. The `api-health-check` workflow now runs every hour automatically.

### Trigger a Webhook

Send a POST request to the webhook path:

```bash
curl -X POST http://localhost:8080/hooks/api-health-check \
  -H "Content-Type: application/json" \
  -d '{"reason": "manual check"}'
```

The request body is available as `trigger.payload` in CEL expressions within the workflow.

### REST API

The server also exposes a REST API for programmatic access:

```bash
# Trigger a workflow
curl -s -X POST http://localhost:8080/api/v1/run/api-health-check | jq .
```

```json
{
  "execution_id": "e5f6a7b8-c9d0-1234-efab-345678901234",
  "workflow": "api-health-check",
  "version": 1
}
```

```bash
# Cancel a running execution
curl -s -X POST http://localhost:8080/api/v1/cancel/e5f6a7b8-c9d0-1234-efab-345678901234
```

Health endpoints are available at `/healthz` (liveness) and `/readyz` (readiness, checks database connectivity). See the [Server Guide](server-guide.md) for production deployment, Helm chart configuration, and graceful shutdown behavior.

## Multi-Tenancy

Mantle supports teams, users, roles, and API keys for multi-tenant environments.

Create a team, add a user, and generate an API key:

```bash
mantle teams create --name acme-corp
```

```
Created team acme-corp (id: f6a7b8c9-d0e1-2345-fabc-456789012345)
```

```bash
mantle users create --email alice@acme.com --name "Alice Chen" --team acme-corp --role admin
```

```
Created user alice@acme.com (role: admin, team: acme-corp)
```

```bash
mantle users api-key --email alice@acme.com --key-name production
```

```
API Key: mk_a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2

Save this key — it cannot be retrieved again.
Key prefix for reference: mk_a1b2c3
```

Available roles are `admin`, `team_owner`, and `operator`. API keys use the `mk_` prefix and are hashed before storage -- the raw key is only shown once at creation time.

This is a brief overview. Multi-tenancy, role-based access control, and team scoping are covered in detail in the [CLI Reference](cli-reference.md).

## Next Steps

You have gone from zero to running workflows with data passing, conditional logic, AI integration, server mode, and multi-tenancy. Here is where to go next:

- **[Workflow Reference](workflow-reference.md)** -- complete YAML schema: every field, every validation rule, every connector (HTTP, AI, Slack, Postgres, Email, S3)
- **[CLI Reference](cli-reference.md)** -- every command, flag, and the REST API
- **[Secrets Guide](secrets-guide.md)** -- credential types, encryption setup, cloud backends (AWS, GCP, Azure), and key rotation
- **[Server Guide](server-guide.md)** -- production deployment, Helm chart, cron and webhook triggers, REST API
- **[Concepts](concepts.md)** -- architecture, checkpointing, CEL expressions, versioning, connectors, plugins, and observability
- **[Plugins Guide](plugins-guide.md)** -- extend Mantle with third-party connector plugins
- **[Observability Guide](observability-guide.md)** -- Prometheus metrics, audit trail, and structured logging
- **[Configuration](configuration.md)** -- config file, environment variables, cloud backends, and flag precedence
- **[examples/](https://github.com/dvflw/mantle/tree/main/examples)** -- ready-to-run workflow files covering HTTP, AI, chained requests, and more
