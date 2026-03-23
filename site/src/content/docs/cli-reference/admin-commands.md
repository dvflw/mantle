# Admin Commands

Commands for secrets management, audit trail, plugins, workflow library, and maintenance.

## mantle secrets create

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

See the [Secrets Guide](/docs/secrets-guide) for credential types, required fields, and usage in workflows.

---

## mantle secrets list

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

## mantle secrets delete

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

## mantle secrets rotate-key

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

## mantle audit

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

## mantle plugins

Manage third-party connector plugins. Plugins are executable binaries that extend Mantle with custom connector actions.

```
Usage:
  mantle plugins list
  mantle plugins install <path>
  mantle plugins remove <name>
```

### mantle plugins list

List all installed plugins in the plugin directory (`.mantle/plugins/` by default).

```bash
$ mantle plugins list
my-custom-connector  .mantle/plugins/my-custom-connector
```

If no plugins are installed:

```
(no plugins installed)
```

### mantle plugins install

Install a plugin by copying the binary into the plugin directory.

```bash
$ mantle plugins install ./build/my-custom-connector
Installed plugin from ./build/my-custom-connector
```

**Arguments:**

| Argument | Required | Description |
|---|---|---|
| `path` | Yes | Path to the plugin binary to install. |

### mantle plugins remove

Remove an installed plugin by name.

```bash
$ mantle plugins remove my-custom-connector
Removed plugin my-custom-connector
```

**Arguments:**

| Argument | Required | Description |
|---|---|---|
| `name` | Yes | Name of the plugin to remove (the filename in the plugins directory). |

See the [Plugins Guide](/docs/plugins-guide) for how to write and test a plugin.

---

## mantle library

Manage the shared workflow template library. Templates let you publish reusable workflow definitions that other teams can deploy.

```
Usage:
  mantle library publish [flags]
  mantle library list
  mantle library deploy [flags]
```

### mantle library publish

Publish a workflow as a shared template. Reads the latest applied version of the named workflow and stores it in the shared library. If a template with the same name already exists, it is updated.

```bash
$ mantle library publish --workflow daily-report
Published "daily-report" to shared library
```

**Flags:**

| Flag | Required | Description |
|---|---|---|
| `--workflow` | Yes | Name of the applied workflow to publish. |

### mantle library list

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

### mantle library deploy

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

## mantle cleanup

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
