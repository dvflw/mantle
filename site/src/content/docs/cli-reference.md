# CLI Reference

Mantle is a single binary with subcommands for every stage of the workflow lifecycle. This page documents every command, its flags, and usage examples.

For installation instructions, see [Getting Started](getting-started.md).

## Global Flags

These flags are available on every command (except where noted):

| Flag | Env Var | Default | Description |
|---|---|---|---|
| `--config` | -- | `mantle.yaml` (in current directory) | Path to the configuration file. |
| `--database-url` | `MANTLE_DATABASE_URL` | `postgres://mantle:mantle@localhost:5432/mantle?sslmode=disable` | Postgres connection URL. |
| `--api-address` | `MANTLE_API_ADDRESS` | `:8080` | API server listen address. |
| `--log-level` | `MANTLE_LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error`. |
| `--api-key` | -- | -- | API key for authentication. Overrides cached credentials from `mantle login`. |
| `--output`, `-o` | -- | `text` | Output format: `text` or `json`. |

Flag precedence, highest to lowest: CLI flags, environment variables, config file, defaults. See [Configuration](configuration.md) for details.

---

## Working Commands

### mantle version

Print version, commit hash, and build date. Does not require a database connection or config file.

```
Usage:
  mantle version
```

**Example:**

```bash
$ mantle version
mantle v0.1.0 (abc32ad, built 2026-03-18T00:00:00Z)
```

---

### mantle init

Run all pending database migrations to set up or upgrade the Mantle schema. This is the first command you run after installing Mantle and starting Postgres.

```
Usage:
  mantle init
```

**Example:**

```bash
$ mantle init
Running migrations...
Migrations complete.
```

**Errors:**

If Postgres is not running or the connection URL is wrong, you see:

```
Error: failed to connect to database: ...
```

Fix: verify Postgres is running (`docker-compose up -d`) and check your `--database-url` or `MANTLE_DATABASE_URL`.

---

### mantle migrate

Run pending database migrations. Functionally equivalent to `mantle init` but organized as a command group with subcommands for status and rollback.

```
Usage:
  mantle migrate
  mantle migrate status
  mantle migrate down
```

#### mantle migrate status

Show which migrations have been applied and which are pending.

```bash
$ mantle migrate status
Applied At                      Migration
============================================================
2026-03-18 10:00:00 +0000       001_initial_schema.sql
```

#### mantle migrate down

Roll back the most recently applied migration. Use with caution in production.

```bash
$ mantle migrate down
Rollback complete.
```

---

### mantle validate

Check a workflow YAML file for structural errors. This command is fully offline -- it does not connect to a database or make any network requests. Global config flags are ignored.

```
Usage:
  mantle validate <file>
```

**Arguments:**

| Argument | Required | Description |
|---|---|---|
| `file` | Yes | Path to the workflow YAML file. |

**Example (valid workflow):**

```bash
$ mantle validate workflow.yaml
workflow.yaml: valid
```

**Example (invalid workflow):**

```bash
$ mantle validate bad-workflow.yaml
bad-workflow.yaml:1:1: error: name must match ^[a-z][a-z0-9-]*$ (name)
bad-workflow.yaml: error: at least one step is required (steps)
```

Errors include file name, line number, column number, message, and the field path. The command exits with code 1 if any validation errors are found.

See the [Workflow Reference](workflow-reference.md) for a complete list of validation rules.

---

### mantle apply

Validate a workflow definition and store it as a new immutable version in the database. If the content has not changed since the last applied version, no new version is created.

```
Usage:
  mantle apply <file>
```

**Arguments:**

| Argument | Required | Description |
|---|---|---|
| `file` | Yes | Path to the workflow YAML file. |

**Example (first apply):**

```bash
$ mantle apply workflow.yaml
Applied fetch-and-summarize version 1
```

**Example (no changes):**

```bash
$ mantle apply workflow.yaml
No changes — fetch-and-summarize is already at version 1
```

**Example (updated workflow):**

```bash
$ mantle apply workflow.yaml
Applied fetch-and-summarize version 2
```

The `apply` command:

1. Reads and parses the YAML file
2. Runs the same validation as `mantle validate`
3. Computes a SHA-256 hash of the file content
4. Compares the hash against the latest stored version
5. If the content changed, inserts a new version row in `workflow_definitions`

Requires a database connection. If validation fails, nothing is written to the database.

---

### mantle plan

Diff a local workflow file against the version stored in the database. Shows what will change before you apply. Runs validation first -- if the file is invalid, the diff is not shown.

```
Usage:
  mantle plan <file>
```

**Arguments:**

| Argument | Required | Description |
|---|---|---|
| `file` | Yes | Path to the workflow YAML file. |

**Example (new workflow):**

```bash
$ mantle plan workflow.yaml
+ fetch-and-summarize (new)

Plan: 1 workflow to create
```

**Example (changed workflow):**

```bash
$ mantle plan workflow.yaml
~ fetch-and-summarize (version 1 → 2)
  ~ steps[1].params.model: gpt-4 → gpt-4o

Plan: 1 workflow to update (version 1 → 2)
```

**Example (no changes):**

```bash
$ mantle plan workflow.yaml
No changes — fetch-and-summarize is at version 2
```

Requires a database connection.

---

### mantle run

Execute a workflow by name. Uses the latest applied version.

```
Usage:
  mantle run <workflow> [flags]
```

**Arguments:**

| Argument | Required | Description |
|---|---|---|
| `workflow` | Yes | Name of the workflow to run. Must have been previously applied. |

**Flags:**

| Flag | Description |
|---|---|
| `--input KEY=VALUE` | Pass an input parameter. Can be specified multiple times. |

**Example:**

```bash
$ mantle run fetch-and-summarize --input url=https://api.example.com/data
Running fetch-and-summarize (version 2)...
Execution abc123-def456: completed
  fetch-data: completed
  summarize: completed
```

If the workflow references a credential and you have `MANTLE_ENCRYPTION_KEY` configured, the engine uses the Postgres-backed credential resolver. Without the encryption key, credentials fall back to environment variables only.

**Errors:**

If the workflow has not been applied:

```
Error: workflow "my-workflow" not found — have you run 'mantle apply'?
```

---

### mantle cancel

Cancel a running or pending workflow execution. Marks the execution and any in-progress steps as cancelled.

```
Usage:
  mantle cancel <execution-id>
```

**Arguments:**

| Argument | Required | Description |
|---|---|---|
| `execution-id` | Yes | UUID of the execution to cancel. |

**Example:**

```bash
$ mantle cancel abc123-def456
Cancelled execution abc123-def456
```

If the execution has already finished:

```bash
$ mantle cancel abc123-def456
Execution abc123-def456 is already completed
```

---

### mantle logs

View execution logs. When called with an execution ID, shows step-by-step detail. When called without arguments, lists recent executions with optional filters.

```
Usage:
  mantle logs [execution-id] [flags]
```

**Arguments:**

| Argument | Required | Description |
|---|---|---|
| `execution-id` | No | UUID of a specific execution. When omitted, lists recent executions. |

**Flags (list mode only):**

| Flag | Default | Description |
|---|---|---|
| `--workflow` | -- | Filter by workflow name. |
| `--status` | -- | Filter by status: `pending`, `running`, `completed`, `failed`, `cancelled`. |
| `--since` | -- | Show executions started within this duration. Accepts Go durations (`1h`, `30m`) and day notation (`7d`). |
| `--limit` | `20` | Maximum number of executions to show. |

**Example -- detail mode (with execution ID):**

```bash
$ mantle logs abc123-def456
Execution: abc123-def456
Workflow:  fetch-and-summarize (version 2)
Status:    completed
Started:   2026-03-18T14:30:00Z
Completed: 2026-03-18T14:30:05Z
Duration:  5.123s

Steps:
  fetch-data           completed (1.2s)
  summarize            completed (3.9s)
```

If a step failed, the error is shown below the step:

```
Steps:
  fetch-data           failed (0.5s)
    error: ai/completion: API returned 401: Unauthorized
```

**Example -- list mode (without arguments):**

```bash
$ mantle logs
ID                                     WORKFLOW             VERSION STATUS     STARTED              COMPLETED
----------------------------------------------------------------------------------------------------------------------------
a1b2c3d4-e5f6-7890-abcd-ef1234567890  fetch-and-summarize        2 completed  2026-03-18 14:30:00  2026-03-18 14:30:05
b2c3d4e5-f6a7-8901-bcde-f12345678901  hello-world                1 completed  2026-03-18 14:28:00  2026-03-18 14:28:01

2 execution(s) shown.
```

**Example -- filtered list:**

```bash
$ mantle logs --workflow hello-world --status failed --since 24h --limit 10
```

---

### mantle status

View the current state of a workflow execution with a summary of step statuses.

```
Usage:
  mantle status <execution-id>
```

**Arguments:**

| Argument | Required | Description |
|---|---|---|
| `execution-id` | Yes | UUID of the execution. |

**Example:**

```bash
$ mantle status abc123-def456
Execution: abc123-def456
Workflow:  fetch-and-summarize (version 2)
Status:    completed
Started:   2026-03-18T14:30:00Z
Completed: 2026-03-18T14:30:05Z

Steps:
  completed: 2
```

---

### mantle secrets create

Create a new encrypted credential. Requires `MANTLE_ENCRYPTION_KEY` or `encryption.key` to be configured.

```
Usage:
  mantle secrets create [flags]
```

**Flags:**

| Flag | Required | Description |
|---|---|---|
| `--name` | Yes | Credential name. Used to reference the credential in workflow steps. |
| `--type` | Yes | Credential type: `generic`, `bearer`, `openai`, `basic`. |
| `--field KEY=VALUE` | Yes | Field value. Repeat for each field the credential type requires. |

**Example:**

```bash
$ mantle secrets create --name my-openai --type openai \
    --field api_key=sk-proj-abc123 \
    --field org_id=org-xyz789
Created credential "my-openai" (type: openai)
```

See the [Secrets Guide](secrets-guide.md) for credential types, required fields, and usage in workflows.

---

### mantle secrets list

List all stored credentials. Shows name, type, and creation date. Never displays decrypted values.

```
Usage:
  mantle secrets list
```

**Example:**

```bash
$ mantle secrets list
NAME        TYPE    CREATED
my-openai   openai  2026-03-18 14:30:00
my-api      basic   2026-03-18 14:35:00
```

---

### mantle secrets delete

Permanently delete a credential by name.

```
Usage:
  mantle secrets delete [flags]
```

**Flags:**

| Flag | Required | Description |
|---|---|---|
| `--name` | Yes | Name of the credential to delete. |

**Example:**

```bash
$ mantle secrets delete --name my-openai
Deleted credential "my-openai"
```

---

### mantle secrets rotate-key

Re-encrypt all stored credentials with a new master key. Use this for key rotation after a security incident or as part of a periodic rotation policy.

```
Usage:
  mantle secrets rotate-key [flags]
```

**Flags:**

| Flag | Required | Description |
|---|---|---|
| `--new-key` | No | Hex-encoded 32-byte new encryption key. If omitted, a new key is auto-generated. |

**Example:**

```bash
$ mantle secrets rotate-key
Re-encrypted 3 credential(s).
New key: a1b2c3d4...
Update MANTLE_ENCRYPTION_KEY to the new key and restart.
```

After rotating, update `MANTLE_ENCRYPTION_KEY` (or `encryption.key` in `mantle.yaml`) to the new key before running any other commands.

---

### mantle serve

Start Mantle as a persistent server with an HTTP API, cron scheduler, and webhook listener. This is the primary way to run Mantle in production.

```
Usage:
  mantle serve [flags]
```

**Flags:**

| Flag | Env Var | Default | Description |
|---|---|---|---|
| `--api-address` | `MANTLE_API_ADDRESS` | `:8080` | Listen address for the HTTP server. |

**Behavior:**

When you run `mantle serve`, Mantle:

1. Runs all pending database migrations automatically
2. Starts the HTTP API on the configured address
3. Starts the cron scheduler, polling every 30 seconds for due triggers
4. Registers webhook listener endpoints for all applied workflows with webhook triggers
5. Serves health endpoints at `/healthz` and `/readyz`

**Example:**

```bash
$ mantle serve
Running migrations...
Migrations complete.
Starting server on :8080
Cron scheduler started (poll interval: 30s)
```

**Custom address:**

```bash
$ mantle serve --api-address :9090
Starting server on :9090
```

**Graceful shutdown:**

Mantle shuts down gracefully on `SIGTERM` or `SIGINT`. When the signal is received:

1. The HTTP server stops accepting new connections
2. In-flight requests are allowed to complete
3. Running workflow executions finish their current step and checkpoint
4. The process exits with code 0

```bash
$ mantle serve
Starting server on :8080
^C
Shutting down gracefully...
Server stopped.
```

**REST API endpoints:**

The server exposes these API endpoints:

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/v1/run/{workflow}` | Trigger a workflow execution. Returns the execution ID, workflow name, and version. |
| `POST` | `/api/v1/cancel/{execution}` | Cancel a running execution. |
| `POST` | `/hooks/<path>` | Webhook trigger endpoint. The request body is available as `trigger.payload` in the workflow's inputs. |
| `GET` | `/healthz` | Liveness probe. Returns 200 when the process is running. |
| `GET` | `/readyz` | Readiness probe. Returns 200 when the database connection is healthy and migrations are applied. |

**Example -- trigger a workflow via the API:**

```bash
$ curl -s -X POST http://localhost:8080/api/v1/run/fetch-and-summarize | jq .
{
  "execution_id": "abc123-def456",
  "workflow": "fetch-and-summarize",
  "version": 2
}
```

**Example -- cancel an execution via the API:**

```bash
$ curl -s -X POST http://localhost:8080/api/v1/cancel/abc123-def456
```

**Example -- trigger a webhook:**

```bash
$ curl -s -X POST http://localhost:8080/hooks/my-workflow \
    -H "Content-Type: application/json" \
    -d '{"event": "deploy", "repo": "my-app"}'
```

Requires a database connection. See the [Server Guide](server-guide.md) for production deployment guidance.

---

### mantle audit

Query the immutable audit trail. Every state-changing operation in Mantle emits an audit event to Postgres. This command queries those events with optional filters.

```
Usage:
  mantle audit [flags]
```

**Flags:**

| Flag | Default | Description |
|---|---|---|
| `--action` | -- | Filter by action type (e.g., `workflow.applied`, `step.completed`, `execution.cancelled`). |
| `--actor` | -- | Filter by actor (e.g., `cli`, `engine`, a user ID). |
| `--resource` | -- | Filter by resource as `type/id` (e.g., `workflow_definition/my-workflow`). |
| `--since` | -- | Show events within this duration. Accepts Go durations (`1h`, `30m`) and day notation (`7d`). |
| `--limit` | `50` | Maximum number of events to show. |

**Action Types:**

| Action | Description |
|---|---|
| `workflow.applied` | A workflow definition was applied (new version stored). |
| `workflow.executed` | A workflow execution started. |
| `step.started` | A step began executing. |
| `step.completed` | A step finished successfully. |
| `step.failed` | A step failed. |
| `step.skipped` | A step was skipped (due to an `if` condition evaluating to `false`). |
| `execution.cancelled` | An execution was cancelled. |

**Example -- all recent events:**

```bash
$ mantle audit
2026-03-18T14:30:00Z  cli           workflow.applied        workflow_definition/hello-world
2026-03-18T14:30:01Z  engine        workflow.executed        workflow_execution/a1b2c3d4
2026-03-18T14:30:01Z  engine        step.started            step_execution/fetch
2026-03-18T14:30:02Z  engine        step.completed          step_execution/fetch
```

**Example -- filtered:**

```bash
$ mantle audit --action workflow.applied --since 7d --limit 20
$ mantle audit --actor cli --resource workflow_definition/hello-world
```

---

### mantle plugins

Manage third-party connector plugins. Plugins are executable binaries that extend Mantle with custom connector actions.

```
Usage:
  mantle plugins list
  mantle plugins install <path>
  mantle plugins remove <name>
```

#### mantle plugins list

List all installed plugins in the plugin directory (`.mantle/plugins/` by default).

```bash
$ mantle plugins list
my-custom-connector  .mantle/plugins/my-custom-connector
```

If no plugins are installed:

```
(no plugins installed)
```

#### mantle plugins install

Install a plugin by copying the binary into the plugin directory.

```bash
$ mantle plugins install ./build/my-custom-connector
Installed plugin from ./build/my-custom-connector
```

**Arguments:**

| Argument | Required | Description |
|---|---|---|
| `path` | Yes | Path to the plugin binary to install. |

#### mantle plugins remove

Remove an installed plugin by name.

```bash
$ mantle plugins remove my-custom-connector
Removed plugin my-custom-connector
```

**Arguments:**

| Argument | Required | Description |
|---|---|---|
| `name` | Yes | Name of the plugin to remove (the filename in the plugins directory). |

See the [Plugins Guide](plugins-guide.md) for how to write and test a plugin.

---

### mantle teams

Manage teams. Teams are the unit of multi-tenancy in Mantle -- each team has its own workflows, credentials, and users.

```
Usage:
  mantle teams create [flags]
  mantle teams list
  mantle teams delete [flags]
```

#### mantle teams create

Create a new team.

**Flags:**

| Flag | Required | Description |
|---|---|---|
| `--name` | Yes | Team name. |

**Example:**

```bash
$ mantle teams create --name my-team
Created team my-team (id: a1b2c3d4-e5f6-7890-abcd-ef1234567890)
```

#### mantle teams list

List all teams.

```bash
$ mantle teams list
NAME      ID                                    CREATED
my-team   a1b2c3d4-e5f6-7890-abcd-ef1234567890  2026-03-18 14:30:00
default   b2c3d4e5-f6a7-8901-bcde-f12345678901  2026-03-18 10:00:00
```

If no teams exist:

```
(no teams)
```

#### mantle teams delete

Delete a team by name.

**Flags:**

| Flag | Required | Description |
|---|---|---|
| `--name` | Yes | Name of the team to delete. |

**Example:**

```bash
$ mantle teams delete --name my-team
Deleted team "my-team"
```

---

### mantle users

Manage users. Users belong to teams and have a role that controls their permissions.

```
Usage:
  mantle users create [flags]
  mantle users list [flags]
  mantle users delete [flags]
  mantle users api-key [flags]
```

#### mantle users create

Create a new user and assign them to a team with a role.

**Flags:**

| Flag | Required | Default | Description |
|---|---|---|---|
| `--email` | Yes | -- | User email address. |
| `--name` | Yes | -- | User display name. |
| `--team` | No | `default` | Team to add the user to. |
| `--role` | No | `operator` | Role to assign: `admin`, `team_owner`, `operator`. |

**Example:**

```bash
$ mantle users create --email alice@example.com --name "Alice Smith" --team my-team --role team_owner
Created user alice@example.com (role: team_owner, team: my-team)
```

#### mantle users list

List users in a team.

**Flags:**

| Flag | Required | Default | Description |
|---|---|---|---|
| `--team` | No | `default` | Team name to list users for. |

**Example:**

```bash
$ mantle users list --team my-team
EMAIL                NAME          ROLE
alice@example.com    Alice Smith   team_owner
bob@example.com      Bob Jones     operator
```

If no users exist:

```
(no users)
```

#### mantle users delete

Delete a user by email.

**Flags:**

| Flag | Required | Default | Description |
|---|---|---|---|
| `--email` | Yes | -- | Email of the user to delete. |
| `--team` | No | `default` | Team name. |

**Example:**

```bash
$ mantle users delete --email bob@example.com
Deleted user "bob@example.com"
```

#### mantle users api-key

Generate an API key for a user. The key is displayed once and cannot be retrieved again.

**Flags:**

| Flag | Required | Description |
|---|---|---|
| `--email` | Yes | User email. |
| `--key-name` | Yes | A name for the API key (for identification). |

**Example:**

```bash
$ mantle users api-key --email alice@example.com --key-name ci-deploy

API Key: mk_a1b2c3d4e5f6...

Save this key — it cannot be retrieved again.
Key prefix for reference: mk_a1b2
```

---

### mantle roles

Manage user roles.

```
Usage:
  mantle roles assign [flags]
```

#### mantle roles assign

Assign a role to an existing user.

**Flags:**

| Flag | Required | Default | Description |
|---|---|---|---|
| `--email` | Yes | -- | User email. |
| `--role` | Yes | -- | Role to assign: `admin`, `team_owner`, `operator`. |
| `--team` | No | `default` | Team name. |

**Example:**

```bash
$ mantle roles assign --email alice@example.com --role admin
Assigned role "admin" to user "alice@example.com"
```

---

### mantle login

Authenticate with a Mantle server. Supports three authentication methods: OIDC authorization code with PKCE (default), device authorization flow, and API key caching.

Credentials are stored in `~/.mantle/credentials`.

```
Usage:
  mantle login [flags]
```

**Flags:**

| Flag | Description |
|---|---|
| `--api-key` | Authenticate by entering and caching an API key. |
| `--device` | Use the device authorization flow (for headless/SSH environments). |

When neither flag is provided, the default OIDC authorization code + PKCE flow is used. This opens a browser for the identity provider login and listens on a local callback URL.

**Example -- OIDC (default):**

```bash
$ mantle login
Open this URL to authenticate:

  https://auth.example.com/authorize?client_id=...

Waiting for callback...
Login successful! Credentials saved to /home/alice/.mantle/credentials
```

**Example -- device flow:**

```bash
$ mantle login --device
To authenticate, visit:

  https://auth.example.com/device

And enter code: ABCD-1234

Waiting for authorization...
Login successful! Credentials saved to /home/alice/.mantle/credentials
```

**Example -- API key:**

```bash
$ mantle login --api-key
Enter API key: mk_a1b2c3d4e5f6...
API key saved to /home/alice/.mantle/credentials
```

OIDC requires `auth.oidc.issuer_url` and `auth.oidc.client_id` to be configured in `mantle.yaml` or via environment variables.

---

### mantle logout

Remove cached credentials from `~/.mantle/credentials`.

```
Usage:
  mantle logout
```

**Example:**

```bash
$ mantle logout
Credentials removed from /home/alice/.mantle/credentials
```

---

### mantle cleanup

Remove old execution data and audit events based on a retention policy. Uses flag values or falls back to the `retention` section in `mantle.yaml`.

```
Usage:
  mantle cleanup [flags]
```

**Flags:**

| Flag | Default | Description |
|---|---|---|
| `--execution-days` | `0` (use config) | Delete workflow executions older than N days. |
| `--audit-days` | `0` (use config) | Delete audit events older than N days. |

If neither flag is set and no retention config exists, the command does nothing.

**Example:**

```bash
$ mantle cleanup --execution-days 30 --audit-days 90
Deleted 42 workflow execution(s) older than 30 day(s).
Deleted 156 audit event(s) older than 90 day(s).
```

**Example -- using config defaults:**

```bash
$ mantle cleanup
Deleted 12 workflow execution(s) older than 30 day(s).
Deleted 89 audit event(s) older than 90 day(s).
```

---

### mantle library

Manage the shared workflow template library. Templates let you publish reusable workflow definitions that other teams can deploy.

```
Usage:
  mantle library publish [flags]
  mantle library list
  mantle library deploy [flags]
```

#### mantle library publish

Publish a workflow as a shared template. Reads the latest applied version of the named workflow and stores it in the shared library. If a template with the same name already exists, it is updated.

```bash
$ mantle library publish --workflow daily-report
Published "daily-report" to shared library
```

**Flags:**

| Flag | Required | Description |
|---|---|---|
| `--workflow` | Yes | Name of the applied workflow to publish. |

#### mantle library list

List all shared workflow templates.

```bash
$ mantle library list
NAME            DESCRIPTION
daily-report    Generate and email a daily summary report
api-monitor     Hourly API health check with Slack alerts
```

If no templates exist:

```
(no templates)
```

#### mantle library deploy

Deploy a shared template as a workflow definition in the target team. Creates a new version in the workflow_definitions table.

```bash
$ mantle library deploy --template daily-report
Deployed "daily-report" as version 1
```

**Flags:**

| Flag | Required | Description |
|---|---|---|
| `--template` | Yes | Name of the template to deploy. |
| `--team` | No | Target team ID. Defaults to the default team. |

---

## REST API

The `mantle serve` command exposes a REST API for programmatic access to Mantle. All endpoints return JSON.

### Endpoints

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/v1/run/{workflow}` | Trigger a workflow execution. |
| `POST` | `/api/v1/cancel/{execution}` | Cancel a running execution. |
| `GET` | `/api/v1/executions` | List executions with optional filters. |
| `GET` | `/api/v1/executions/{id}` | Get execution details with step information. |
| `POST` | `/hooks/{path}` | Webhook trigger endpoint. |
| `GET` | `/healthz` | Liveness probe. |
| `GET` | `/readyz` | Readiness probe (checks database connectivity). |
| `GET` | `/metrics` | Prometheus metrics endpoint. |

### POST /api/v1/run/{workflow}

Triggers a new execution of the named workflow using the latest applied version.

```bash
curl -s -X POST http://localhost:8080/api/v1/run/fetch-and-summarize | jq .
```

```json
{
  "execution_id": "abc123-def456",
  "workflow": "fetch-and-summarize",
  "version": 2
}
```

### POST /api/v1/cancel/{execution}

Cancels a running or pending execution.

```bash
curl -s -X POST http://localhost:8080/api/v1/cancel/abc123-def456
```

```json
{
  "execution_id": "abc123-def456",
  "status": "cancelled"
}
```

### GET /api/v1/executions

Lists recent executions. Supports query parameters for filtering.

**Query Parameters:**

| Parameter | Type | Description |
|---|---|---|
| `workflow` | string | Filter by workflow name. |
| `status` | string | Filter by status: `pending`, `running`, `completed`, `failed`, `cancelled`. |
| `since` | string | Show executions within this duration (e.g., `1h`, `24h`, `7d`). |
| `limit` | number | Maximum results. Default: `20`. |

```bash
curl -s "http://localhost:8080/api/v1/executions?workflow=hello-world&status=completed&limit=5" | jq .
```

```json
{
  "executions": [
    {
      "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
      "workflow": "hello-world",
      "version": 1,
      "status": "completed",
      "started_at": "2026-03-18T14:30:00Z",
      "completed_at": "2026-03-18T14:30:01Z"
    }
  ]
}
```

### GET /api/v1/executions/{id}

Returns detailed information about a single execution, including all step results.

```bash
curl -s http://localhost:8080/api/v1/executions/a1b2c3d4-e5f6-7890-abcd-ef1234567890 | jq .
```

```json
{
  "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "workflow": "hello-world",
  "version": 1,
  "status": "completed",
  "started_at": "2026-03-18T14:30:00Z",
  "completed_at": "2026-03-18T14:30:01Z",
  "steps": [
    {
      "name": "fetch",
      "status": "completed",
      "started_at": "2026-03-18T14:30:00Z",
      "completed_at": "2026-03-18T14:30:01Z"
    }
  ]
}
```

---

## Exit Codes

| Code | Meaning |
|---|---|
| `0` | Success |
| `1` | Validation error, runtime error, or command failure |
