# Workflow Commands

These commands manage the workflow lifecycle: validation, deployment, execution, and inspection.

## mantle validate

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

See the [Workflow Reference](/docs/workflow-reference) for a complete list of validation rules.

---

## mantle apply

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

## mantle plan

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

## mantle run

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

## mantle cancel

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

## mantle logs

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

## mantle retry

Retry a failed workflow execution. By default, resumes from the first failed step, reusing outputs from previously completed steps.

```text
Usage:
  mantle retry <execution-id> [flags]
```

**Arguments:**

| Argument | Required | Description |
|---|---|---|
| `execution-id` | Yes | UUID of the failed execution to retry. |

**Flags:**

| Flag | Default | Description |
|---|---|---|
| `--from-step` | -- | Step name to retry from. Overrides the default behavior of resuming from the first failed step. All steps from this point forward are re-executed. |
| `--force` | `false` | Bypass per-workflow and per-team concurrency limits. |

**Example -- retry from failed step:**

```bash
$ mantle retry abc123-def456
Retrying execution abc123-def456 from step "summarize"...
Execution def456-abc123: completed
  fetch-data: reused (cached)
  summarize: completed (4.1s)
  post-result: completed (0.3s)
```

**Example -- retry from a specific step:**

```bash
$ mantle retry abc123-def456 --from-step fetch-data
Retrying execution abc123-def456 from step "fetch-data"...
Execution ghi789-jkl012: completed
  fetch-data: completed (1.1s)
  summarize: completed (3.8s)
  post-result: completed (0.2s)
```

**Errors:**

If the execution is not in a failed state:

```text
Error: execution abc123-def456 is not in a failed state (status: completed).
```

If `--from-step` references a step that does not exist:

```text
Error: step "nonexistent" not found in workflow "fetch-and-summarize"
```

---

## mantle rollback

Roll back a workflow to a previous version. The previously active version becomes the current version used by `mantle run` and triggers.

```text
Usage:
  mantle rollback <workflow> [flags]
```

**Arguments:**

| Argument | Required | Description |
|---|---|---|
| `workflow` | Yes | Name of the workflow to roll back. |

**Flags:**

| Flag | Default | Description |
|---|---|---|
| `--to-version` | -- | Specific version number to roll back to. If omitted, rolls back to the previous version (current - 1). |

**Example -- rollback to previous version:**

```bash
$ mantle rollback fetch-and-summarize
Rolled back fetch-and-summarize from version 3 to version 2
```

**Example -- rollback to a specific version:**

```bash
$ mantle rollback fetch-and-summarize --to-version 1
Rolled back fetch-and-summarize from version 3 to version 1
```

**Errors:**

If the workflow has only one version:

```text
Error: workflow "fetch-and-summarize" is at version 1 — nothing to roll back to
```

If the target version does not exist:

```text
Error: workflow "fetch-and-summarize" version 5 not found
```

---

## mantle status

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
