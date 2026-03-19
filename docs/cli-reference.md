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

View step-by-step execution history with timing, status, and errors.

```
Usage:
  mantle logs <execution-id>
```

**Arguments:**

| Argument | Required | Description |
|---|---|---|
| `execution-id` | Yes | UUID of the execution. |

**Example:**

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

## Coming Soon

The following commands are planned but not yet implemented.

### mantle serve

Start Mantle as a persistent server with cron scheduling and webhook ingestion.

```
Usage:
  mantle serve [flags]
```

**Expected flags:**

| Flag | Description |
|---|---|
| `--api-address` | Listen address for the API server (default `:8080`). |

---

## Exit Codes

| Code | Meaning |
|---|---|
| `0` | Success |
| `1` | Validation error, runtime error, or command failure |
