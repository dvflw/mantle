# v0.4.0 — The Safety Net Update: Design Spec

**Milestone:** [v0.4.0](https://github.com/dvflw/mantle/milestone/5)
**Issues:** #52, #75, #51, #49, #30, #48, #50 (7 issues; #70 release pipeline is a separate plan)
**Implementation order:** #52 → #75 → #51 → #49 → #30 → #48 → #50

---

## 1. Config File Versioning (#52)

Add a top-level `version` field to `mantle.yaml` as a guard rail for future config changes.

### Schema

```yaml
version: 1
database:
  url: postgres://...
```

### Behavior on Load

| `version` value | Behavior |
|-----------------|----------|
| Missing or `0` | Treat as `1`, no warning (backward compat for existing installs) |
| `1` | Valid, proceed |
| `2+` | Hard error: `"unsupported config version %d; this version of mantle supports config version 1 — upgrade mantle or check your mantle.yaml"` |

### Changes

- `internal/config/config.go`: Add `Version int` to `Config`, validate in `Load()`. No env var binding — `version` is config-file-only (setting it via `MANTLE_VERSION` would be confusing and error-prone).
- Docs: Update `configuration.md`, update all example configs

### What This Does NOT Do

No migration logic. Version 1 is the only version. The field exists so future config changes can bump the version and old binaries reject new configs cleanly rather than silently zero-valuing new fields.

---

## 2. Rename tmp Config Section to storage (#75)

The `tmp` config section stores execution artifacts (screenshots, PDFs, reports) with configurable retention. These are not temporary files — the naming is misleading.

### Schema Change

```yaml
# Before
tmp:
  type: s3
  bucket: mantle-artifacts
  prefix: artifacts/
  retention: 24h

# After
storage:
  type: s3
  bucket: mantle-artifacts
  prefix: artifacts/
  retention: 24h
```

Environment variables: `MANTLE_TMP_*` → `MANTLE_STORAGE_*`

### Backward Compatibility

Within config version 1: if `storage` is not set but `tmp` is present, emit a deprecation warning via `slog.Warn` and copy the values over. The `tmp` fallback gets removed in a future version bump.

### Changes

- `internal/config/config.go`: Rename `TmpConfig` → `StorageConfig`, `Tmp` → `Storage`, add `tmp` fallback logic with deprecation warning
- `internal/artifact/tmp.go` → `internal/artifact/storage.go`: Rename `TmpStorage` interface → `Storage`, `FilesystemTmpStorage` struct → `FilesystemStorage`
- `internal/engine/engine.go`: Update `TmpStorage` field name
- `internal/cli/`, `internal/server/`: Update references
- `docker-compose.yml`: Update env vars
- Docs: `configuration.md`, `deployment-guide.md`

---

## 3. Secret Key Rotation (#51)

New CLI command to re-encrypt all credentials with a new AES-256-GCM master key.

### Command

```
mantle secrets rotate-key [--new-key <hex-encoded-32-byte-key>]
```

- `--new-key` is **optional**. If omitted, a new 32-byte key is auto-generated and printed to stdout.
- Also accepts `MANTLE_NEW_ENCRYPTION_KEY` env var.
- **Requires admin privileges** (admin API key or admin OIDC user). Non-admin callers get a permission error.

### Flow

1. Verify caller has admin privileges
2. Load current encryption key from config (`encryption.key`)
3. If `--new-key` provided, use it; otherwise generate via `secret.GenerateKey()` and print it
4. Create two `secret.Encryptor` instances (old key, new key)
5. Open a single transaction
6. `SELECT id, name, encrypted_data, nonce FROM credentials` — **all teams, global scope**
7. For each credential: decrypt with old encryptor → re-encrypt with new encryptor (fresh random 12-byte GCM nonce)
8. `UPDATE` each row in place
9. Commit transaction (all-or-nothing — if any credential fails to decrypt, abort and report which credential failed, including both `name` and `id` in the error message)
10. Emit `ActionSecretKeyRotated` audit event with credential count
11. Print: `"Rotated N credentials. Update your mantle.yaml encryption.key and restart."`

### Design Decisions

- **Single transaction:** If any credential fails to decrypt, the whole rotation rolls back. No partial state. The error identifies the specific failed credential.
- **Global scope:** Rotation operates across all teams. The master key is system-wide, not per-team.
- **No automatic config rewrite:** The command tells the user to update their config. Writing config files is error-prone (comments stripped, formatting changes).
- **Algorithm unchanged:** Rotation changes the key, not the cipher. AES-256-GCM throughout.

### Changes

- `internal/cli/secrets_rotate_key.go`: New command under `secrets` subcommand. CLI owns the transaction: begins tx, calls `RotateAll`, commits or rolls back.
- `internal/secret/store.go`: Add `RotateAll(ctx, tx, oldEncryptor, newEncryptor) (int, error)`. This **replaces** the existing `ReEncryptAll` method, removing its `WHERE team_id = $1` filter to operate globally. The caller provides the transaction. Error messages include both credential name and ID.
- `internal/audit/actions.go`: Add `ActionSecretKeyRotated`

---

## 4. Concurrency Controls (#49)

Three concurrency dimensions to prevent execution floods.

### Concurrency Dimensions

| Dimension | Config Location | Default |
|-----------|----------------|---------|
| Per-workflow `max_parallel_executions` | Workflow YAML | `0` (unlimited) |
| Per-team global limit | `mantle.yaml` default + per-team DB override | `0` (unlimited) |
| Per-step `max_parallel` | Step YAML (fan-out steps) | `0` (unlimited) |

### YAML Schema

```yaml
name: my-workflow
max_parallel_executions: 5
on_limit: queue    # "queue" (default) | "reject"

steps:
  - name: process-batch
    action: http/request
    depends_on: [split-data]
    max_parallel: 3
    params: ...
```

`mantle.yaml`:
```yaml
engine:
  max_concurrent_executions_per_team: 50
```

### New Execution Status

```
queued → pending → running → {completed | failed | cancelled}
```

- `queued` = waiting for a concurrency slot
- `cancelled` works from both `queued` and `running` states
- `queued → cancelled` must set `completed_at` (same as other terminal transitions)

**Code paths that need `queued` awareness:**
- `createExecution` must support inserting as `queued` (currently always inserts as `pending`)
- `updateExecutionStatus` terminal check (engine.go) must recognize `queued` as a valid non-terminal state
- `mantle cancel` must handle cancelling `queued` executions (no steps to cancel, just status update)
- `mantle status` must display `queued` status

### Enforcement — Advisory Locks

Execution creation wraps in a transaction:

1. `pg_advisory_xact_lock(hash(team_id))` → count running executions for team → check per-team limit
2. `pg_advisory_xact_lock(hash(workflow_name))` → count running executions for workflow → check per-workflow limit
3. Under limits → insert with `status = 'pending'`
4. Over limits, `on_limit = queue` → insert with `status = 'queued'`
5. Over limits, `on_limit = reject` → return error (webhook returns 429)

### Queue Promotion

- **Primary (completion-triggered):** When an execution reaches a terminal state, `promoteQueued()` checks for the oldest `queued` execution for the same workflow and promotes it to `pending`.
- **Backup (30s poller):** Scans for queued executions with available slots. Catches crashes during promotion.

### Fan-Out max_parallel in ReadySteps

`ReadySteps` (orchestrator.go) currently returns all steps whose deps are resolved. For steps that share a fan-out parent, count how many are currently `running` and only release up to `max_parallel`. Requires passing `MaxParallel` into the `workflowStep` struct.

### Webhook Behavior

- `on_limit: queue` (default) → 202 Accepted, execution created as `queued`
- `on_limit: reject` → 429 Too Many Requests, no execution created

### CLI Behavior

- `mantle run` respects concurrency limits by default
- `mantle run --force` bypasses limits (operator override)

### Migration

- Add `max_concurrent_executions INT` nullable column to `teams` table (overrides config default)
- No changes to `workflow_definitions` — concurrency fields live in the YAML content stored as JSONB

### Struct Changes

- `workflow.Workflow`: Add `MaxParallelExecutions int`, `OnLimit string`. `OnLimit` zero value (`""`) is treated as `"queue"` during enforcement — the default is applied in the enforcement logic, not during parsing.
- `workflow.Step`: Add `MaxParallel int`
- `config.EngineConfig`: Add `MaxConcurrentExecutionsPerTeam int`

### Metrics

- `mantle_executions_queued` (gauge) — current queue depth per workflow
- `mantle_executions_rejected_total` (counter) — rejected due to limit
- `mantle_queue_wait_duration_seconds` (histogram) — time in queue before promotion

### Edge Cases

- Any terminal state frees a concurrency slot
- Orphaned `running` executions after crash count against limits temporarily — accepted tradeoff; reaper handles recovery, backup poller covers queue promotion
- `0` means unlimited for all three dimensions

---

## 5. on_failure Workflow Lifecycle Hooks (#30)

Three lifecycle hook blocks that execute after main workflow steps complete.

### Hook Blocks

| Hook | Fires when | Use case |
|------|-----------|----------|
| `on_success` | Main workflow completes with no unhandled failures | Success notifications, downstream triggers |
| `on_failure` | Main workflow step fails (without `continue_on_error`) or workflow times out | Alerts, cleanup, incident creation |
| `on_finish` | Always — success, failure, or timeout (but NOT cancellation) | Resource cleanup, audit logging |

### YAML Schema

```yaml
name: my-workflow
timeout: 5m
steps:
  - name: fetch-data
    action: http/request
    params:
      url: https://api.example.com/data

hooks:
  timeout: 2m
  on_success:
    - name: notify-team
      action: slack/send
      credential: slack-bot
      params:
        channel: "#ops"
        text: "Workflow completed successfully"
  on_failure:
    - name: alert-ops
      action: slack/send
      credential: slack-bot
      params:
        channel: "#ops-alerts"
        text: "Failed at {{ execution.failed_step }}: {{ execution.error }}"
  on_finish:
    - name: cleanup
      action: http/request
      params:
        method: POST
        url: "https://api.example.com/cleanup"
```

### Execution Order (Fixed)

```
Main workflow succeeds → on_success → on_finish
Main workflow fails    → on_failure → on_finish
Main workflow timeout  → on_failure → on_finish
Main workflow cancel   → no hooks
```

1. Conditional hook runs first (`on_success` OR `on_failure`, never both from main workflow)
2. `on_finish` always runs last (except on cancellation)

### Struct Changes

Add `Hooks *HooksConfig` to `workflow.Workflow`:

```go
type HooksConfig struct {
    Timeout   string `yaml:"timeout"`
    OnSuccess []Step `yaml:"on_success"`
    OnFailure []Step `yaml:"on_failure"`
    OnFinish  []Step `yaml:"on_finish"`
}
```

Hook steps reuse the existing `Step` type with full feature support: `credential`, `timeout`, `retry`, `if`, `continue_on_error`, `artifacts`, `registry_credential`.

### Hook Step Behavior

- Flat list, executed sequentially — no `depends_on`
- Names unique within their block, not globally (can share names with main steps). The `hook_block` column provides disambiguation in `step_executions` — queries must always filter on `hook_block IS NULL` for main steps or `hook_block = $2` for hook steps.
- Hook step fails → halt remaining steps in that block → next block still runs
- `continue_on_error: true` on a hook step → record failure, continue to next step in block
- `on_finish` always runs regardless of conditional block outcome
- No cascading: `on_success` failure does NOT trigger `on_failure`
- Hook failures are logged, audited, and emitted as metrics
- Hook failures never alter the workflow execution status

### CEL Context in Hooks

| Variable | Description |
|----------|-------------|
| `steps['name'].output` | Main workflow step outputs |
| `steps['name'].error` | Main workflow step errors |
| `hooks['name'].output` | Hook step outputs (accumulates across blocks) |
| `hooks['name'].error` | Hook step errors |
| `execution.status` | `"completed"`, `"failed"`, or `"timed_out"` |
| `execution.error` | Error string or `null` |
| `execution.failed_step` | Name of the step that caused failure, or `null` |
| `execution.failed_in` | `"steps"`, `"on_success"`, `"on_failure"`, `"on_finish"`, or `null` |
| `inputs`, `trigger`, `env`, `artifacts` | Same as main workflow steps |

### Engine Changes

After `resumeExecution` completes the main steps, a new `executeHooks` method runs:

1. Determine main workflow outcome: `completed`, `failed`, or `timed_out`
2. Build execution context CEL variables (`execution.status`, etc.)
3. Run the conditional block (`on_success` or `on_failure`)
4. Run `on_finish`
5. Each block respects `hooks.timeout` as an aggregate cap

### Timeouts

- `timeout` (workflow-level, existing) — covers main steps only
- `hooks.timeout` (optional) — separate aggregate cap for all hook execution. The hook timeout starts fresh when hooks begin executing, regardless of how much time the main workflow consumed. If the main workflow times out, hooks get their full `hooks.timeout` budget.
- Individual hook steps can have their own `timeout` (existing field)

### DB Representation

Hook step executions go into `step_executions` with a `hook_block TEXT` column:
- `NULL` for main steps
- `"on_success"`, `"on_failure"`, or `"on_finish"` for hook steps

This keeps existing claim/lease/reaper infrastructure working for hook steps.

**Important:** `GetStepStatuses` in `orchestrator.go` must add `AND hook_block IS NULL` to its query filter. Without this, hook steps would pollute the DAG-based `AdvanceExecution` logic. Hook step statuses are queried separately by the `executeHooks` method using `AND hook_block = $2`.

### Validation Additions

- Hook step names unique within their block
- No `depends_on` in hook steps (reject with clear error)
- `hooks.timeout` is valid duration if present

### Metrics

- `mantle_hook_steps_total{workflow, hook, step, status}` counter
- `mantle_hook_steps_failed_total{workflow, hook, step}` counter

### Concurrency Interaction

The concurrency slot (from #49) is held from `running` through hook completion. Queue promotion fires when the execution reaches its terminal state, which is after all hooks complete. This prevents a queued execution from starting while the previous execution's cleanup hooks are still running.

### What This Does NOT Do

- Hooks do not cascade (`on_success` failure does not trigger `on_failure`)
- Hooks do not alter workflow execution status
- Hooks do not support `depends_on` (use child workflows for complex error handling)
- `continue_on_error` workflows that "complete" with step errors do NOT trigger `on_failure`

---

## 6. Retry from Failed Step (#48)

New CLI command to resume execution from the point of failure.

### Command

```
mantle retry <execution-id> [--from-step <step-name>] [--force]
```

- `--from-step` retries from a specific step regardless of its status (failed, cancelled, or completed). If omitted, retries from the first failed step in topological order.
- `--force` bypasses concurrency limits (consistent with `mantle run --force`).

### Flow

1. Load original execution from `workflow_executions` (workflow name, version, inputs)
2. Load all step statuses via `GetStepStatuses`
3. If `--from-step` provided, validate step exists; otherwise find the first failed step in topological order
4. Create a **new execution** (new UUID, same workflow version, same inputs)
5. Copy completed step outputs from the original execution for all steps preceding the retry point — insert as `completed` step_executions in the new execution. **Do not copy hook steps** (`hook_block IS NOT NULL`); hooks fire fresh based on the new outcome.
6. Mark the retry-point step and all downstream steps as `pending`
7. Execute the new workflow via `Engine.Execute()` (respects concurrency limits unless `--force`)
8. Emit `ActionExecutionRetried` audit event with execution ID, target step, and operator

### Design Decisions

- **New execution, not resume:** The old execution is a completed audit record. Creating a new execution with `retried_from` metadata preserves the audit trail.
- **Same version:** Retry uses the workflow version from the original execution, not the latest version. Users who want the new definition should `mantle run` fresh.
- **Hooks fire fresh:** Since this is a new execution, hooks run based on the new outcome, not the original.

### DB Changes

- Add `retried_from_execution_id UUID` nullable column to `workflow_executions`

### CLI Output

Same as `mantle run` — streams logs, returns final status.

### Changes

- `internal/cli/retry.go`: New command
- `internal/engine/engine.go`: Add `RetryExecution(ctx, originalExecID, fromStep)` method
- `internal/audit/actions.go`: Add `ActionExecutionRetried`

---

## 7. Workflow Rollback (#50)

New CLI command to revert a workflow to a previous version.

### Command

```
mantle rollback <workflow> [--to-version <N>]
```

### Flow

1. If `--to-version` provided, use it; otherwise default to the **second most recent version** (not literal `current - 1`, since version numbers may be non-contiguous due to prior rollbacks)
2. Validate target version exists in `workflow_definitions`
3. Reject version `0` or negative values
4. Load target version's content
5. Validate target version content differs from current (reject no-op rollback with message)
6. Insert a **new version row**: `version = current_max + 1`, content = target version's content, `rollback_of = target_version`
7. If running in server mode, reload triggers for the updated workflow
8. Emit `ActionWorkflowRolledBack` audit event with from/to version info
9. Print: `"Rolled back <workflow> from version N to content of version M (now version N+1)"`

### Design Decisions

- **New version, not pointer:** The engine always runs the latest version. Creating a new row with old content preserves full version history and audit trail.
- **`mantle plan` shows no diff** after rollback (content matches what's deployed).
- **In-flight executions unaffected:** Running executions use their original version. The rollback only affects new executions. Rollback is **not blocked** by running executions.
- **Trigger reload:** Server mode must pick up trigger changes (cron schedules, webhook paths) from the rolled-back content.
- **Concurrency interaction:** Rollback does not interact with concurrency controls — it only inserts a new version row.

### DB Changes

- Add `rollback_of INT` nullable column to `workflow_definitions` (points to the version number whose content was restored, for traceability)

### Changes

- `internal/cli/rollback.go`: New command
- `internal/audit/actions.go`: Add `ActionWorkflowRolledBack`

---

## Migration Summary

Single migration file `016_safety_net.sql` covering all DB changes:

```sql
-- Concurrency controls (#49)
ALTER TABLE teams ADD COLUMN max_concurrent_executions INT;

-- Retry from failed step (#48)
ALTER TABLE workflow_executions ADD COLUMN retried_from_execution_id UUID
  REFERENCES workflow_executions(id);

-- Lifecycle hooks (#30)
ALTER TABLE step_executions ADD COLUMN hook_block TEXT;

-- Workflow rollback (#50)
ALTER TABLE workflow_definitions ADD COLUMN rollback_of INT;
```

---

## Implementation Order Rationale

1. **#52 Config versioning** + **#75 Rename tmp→storage** — Low-risk config changes. #52 provides the versioning guard rail; #75 ships the breaking rename with deprecation fallback. Both touch `config.go`.
2. **#51 Secret key rotation** — Standalone security feature. No dependencies on other issues.
3. **#49 Concurrency controls** — Adds `queued` status and changes the execution creation path. Must land before hooks so hooks understand the full status lifecycle.
4. **#30 on_failure hooks** — Largest feature. Needs to know about all execution statuses including `queued` from #49.
5. **#48 Retry** + **#50 Rollback** — Additive CLI commands. Benefit from the updated execution model but don't change the core engine path.
