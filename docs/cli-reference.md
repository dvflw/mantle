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

## Coming Soon

The following commands are planned but not yet implemented.

### mantle plan

Diff a local workflow file against the version stored in the database. Shows what will change before you apply.

```
Usage:
  mantle plan <file>
```

**Expected behavior:**

```bash
$ mantle plan workflow.yaml
~ fetch-and-summarize (version 1 → 2)
  ~ steps[1].params.model: gpt-4 → gpt-4o
```

---

### mantle run

Execute a workflow by name. Uses the latest applied version.

```
Usage:
  mantle run <workflow> [flags]
```

**Expected flags:**

| Flag | Description |
|---|---|
| `--input KEY=VALUE` | Pass an input parameter. Repeat for multiple inputs. |
| `--version N` | Run a specific version instead of the latest. |

**Expected usage:**

```bash
$ mantle run fetch-and-summarize --input url=https://api.example.com/data
Started execution abc123-def456
```

---

### mantle cancel

Cancel a running workflow execution.

```
Usage:
  mantle cancel <execution-id>
```

---

### mantle logs

View execution logs for a workflow run.

```
Usage:
  mantle logs <execution-id>
```

---

### mantle status

View the current state of a workflow execution, including step-level status.

```
Usage:
  mantle status <execution-id>
```

---

### mantle secrets create

Create a typed credential for use in workflows. Secrets are stored encrypted and resolved at connector invocation time.

```
Usage:
  mantle secrets create [flags]
```

---

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
