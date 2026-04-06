# Workflow Composition — workflow/run Action: Design Spec

**Issue:** #54
**Milestone:** v0.5.0 — The GitOps Update

---

## Summary

Add a `workflow/run` connector that lets one workflow invoke another synchronously. Child execution blocks until complete, output is wrapped and accessible to the parent, cancellation cascades, and token budgets are inherited or capped.

## YAML Schema

```yaml
- name: send-notification
  action: workflow/run
  params:
    workflow: notification-sender
    version: 3              # optional, defaults to latest
    inputs:
      channel: "{{ steps['classify'].output.json.category }}"
      message: "{{ steps['classify'].output.json.summary }}"
    token_budget: 5000      # optional, defaults to parent's remaining budget
```

## Output

The step output wraps the child's full execution result:

```json
{
  "execution_id": "abc-123",
  "status": "completed",
  "steps": {
    "step-1": { "output": { ... } },
    "step-2": { "output": { ... } }
  }
}
```

Parent accesses child results: `steps['send-notification'].output.steps['send-email'].output.message_id`

## Migration (016 on main, 017 if v0.4.0 lands first)

```sql
ALTER TABLE workflow_executions ADD COLUMN parent_execution_id UUID REFERENCES workflow_executions(id);
ALTER TABLE workflow_executions ADD COLUMN parent_step_name TEXT;
ALTER TABLE workflow_executions ADD COLUMN depth INTEGER NOT NULL DEFAULT 0;
CREATE INDEX idx_workflow_executions_parent ON workflow_executions(parent_execution_id) WHERE parent_execution_id IS NOT NULL;
```

## Engine Config

```yaml
engine:
  max_workflow_depth: 10  # default
```

Ensure `EngineConfig.MaxWorkflowDepth` uses default 10 and is bound to env/config:

```go
v.SetDefault("engine.max_workflow_depth", 10)
_ = v.BindEnv("engine.max_workflow_depth", "MANTLE_ENGINE_MAX_WORKFLOW_DEPTH")
```

## Connector Implementation

New file: `internal/connector/workflow.go`

The `workflow/run` connector is special — it needs access to the `Engine` to execute the child workflow. Unlike other connectors which are stateless functions, this one needs a reference back to the engine.

**Approach:** The `workflow/run` connector cannot be registered inside `NewRegistry()` because it needs an `*Engine` reference, which doesn't exist yet at registry construction time. Instead, register it after the engine is constructed — in the CLI commands (run.go, serve.go) or in a new `Engine.RegisterBuiltinConnectors()` method. The connector struct holds an engine pointer:

```go
type WorkflowConnector struct {
    engine *Engine // set after engine construction
}
```

This avoids circular imports between `engine` and `connector` packages.

**Execute logic:**
1. Resolve `params.workflow` name and optional `params.version` (latest if omitted)
2. Check depth: query parent's depth from DB, reject if `depth + 1 > max_workflow_depth`
3. Resolve inputs via CEL (already handled by the engine's param resolution)
4. Call `engine.Execute()` (or a variant) with:
   - The child workflow name and version
   - The resolved inputs
   - Parent's team context (same team_id)
   - `parent_execution_id` and `parent_step_name` set on the new execution
   - Token budget: `params.token_budget` if set, else parent's remaining budget
5. Return the wrapped `ExecutionResult` as the step output

## Recursion Safety

- `max_workflow_depth` checked at child creation time
- Depth stored on `workflow_executions.depth` column
- Self-recursion is allowed (depth limit is the only guard)
- Error message: `"workflow depth limit exceeded (%d/%d)"` with current depth and max

## Cancellation Cascade

When `mantle cancel <parent-id>` is called:
- Cancel the parent execution (existing behavior)
- Cancel the entire execution tree atomically using a recursive CTE:

```sql
WITH RECURSIVE children AS (
  SELECT id FROM workflow_executions WHERE id = $1
  UNION ALL
  SELECT e.id FROM workflow_executions e
  JOIN children c ON e.parent_execution_id = c.id
)
UPDATE workflow_executions SET status = 'cancelled', completed_at = NOW(), updated_at = NOW()
WHERE id IN (SELECT id FROM children) AND status IN ('pending', 'running', 'queued')
```

This handles arbitrarily deep trees atomically, avoiding TOCTOU races where new children could be created between loop iterations.

Update `internal/cli/cancel.go` to use this CTE.

## Checkpoint Recovery

On crash recovery (parent resumes from checkpoint):
- Before creating a new child execution, check if one already exists for this `parent_execution_id + parent_step_name`
- If found and still running/pending, resume it (wait for completion)
- If found and completed, use its result (skip re-execution)
- If found and failed, treat as step failure (don't re-create)

This is handled in the connector's Execute logic by querying for existing child executions first.

## Token Budget

- Child inherits parent's context (including `StepContext.WorkflowTokenBudget`)
- If `params.token_budget` is set, use `min(params.token_budget, parent_remaining)` as the child's budget
- Child's AI token usage counts against whatever budget it was given

## Credential Scoping

- Child runs in parent's team context (same `team_id` from context)
- Credentials resolve against the calling team's store
- No cross-team credential access (enforced by existing team_id scoping)

## CLI Enhancements

### `mantle logs <execution-id>`
- When displaying steps, check if a step used `workflow/run`
- If so, query child execution by `parent_execution_id + parent_step_name`
- Display child steps indented under the parent step
- `--shallow` flag suppresses child details

### `mantle status <execution-id>`
- Show execution tree: parent → children with status

## Feature Interactions

- **continue_on_error:** Child failure = step failure. Parent continues if `continue_on_error: true`.
- **Retry policies:** Parent step retry re-invokes child from scratch (new child execution).
- **Concurrency controls:** Child execution counts against per-workflow limits for the *child* workflow name.
- **Lifecycle hooks:** Child hooks run within the child execution. Parent hooks run after parent completes.
- **Artifacts:** Child artifacts are accessible in the wrapped output.

## Changes Summary

| Action | File | Responsibility |
|--------|------|----------------|
| Create | `internal/db/migrations/017_workflow_composition.sql` | Schema changes |
| Create | `internal/connector/workflow.go` | workflow/run connector |
| Modify | `internal/connector/connector.go` | Register workflow/run |
| Modify | `internal/config/config.go` | Add MaxWorkflowDepth |
| Modify | `internal/engine/engine.go` | ExecuteChild method, depth tracking |
| Modify | `internal/cli/cancel.go` | Cascade cancellation to children |
| Modify | `internal/cli/logs.go` | Show child logs inline |
| Modify | `internal/cli/status.go` | Show execution tree |
| Modify | `internal/audit/audit.go` | Add ActionChildWorkflowExecuted |
| Update | Docs: workflow-reference, connectors | Document workflow/run |

## What This Does NOT Do

- Async invocation (fire-and-forget, fan-out/fan-in of child workflows)
- Cross-team workflow calls
- Input validation at invocation time (child validates its own inputs)
