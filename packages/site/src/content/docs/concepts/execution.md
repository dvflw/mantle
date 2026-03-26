# Execution Model

This page covers how Mantle executes workflows: checkpointing, crash recovery, parallel execution, and AI tool use.

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

## Parallel Execution

Mantle does not execute steps strictly in list order. Instead, it builds a directed acyclic graph (DAG) from both explicit `depends_on` declarations and implicit dependencies detected from CEL expression analysis (e.g., `steps.fetch-data.output`), then schedules steps for concurrent execution when their dependencies allow it.

**How it works:**

1. Steps with no dependencies start immediately and run in parallel.
2. When a step completes (or is skipped), the engine checks which downstream steps now have all dependencies resolved and starts them.
3. If a step fails, all downstream steps that transitively depend on it are cancelled.

**Implicit dependency detection:** When you reference `steps['fetch-data'].output.body` in a step's `params` or `if` field, the engine automatically adds `fetch-data` as a dependency. You do not need to redundantly list it in `depends_on`.

**Skipped steps count as resolved.** If a step's `if` condition evaluates to `false`, it is marked as skipped. Downstream steps that depend on it are still unblocked -- they can proceed, though referencing the skipped step's output will yield an empty value.

For the full `depends_on` field reference and a fan-out/fan-in YAML example, see the [Workflow Reference](/docs/workflow-reference#parallel-execution).

## AI Tool Use

The `ai/completion` connector supports multi-turn tool use (function calling). You declare tools in the step's `params`, each mapping a tool name to a Mantle connector action. The engine then orchestrates a loop between the LLM and your tools:

1. The engine sends the prompt (and tool definitions) to the LLM.
2. The LLM responds with either a text completion or one or more tool call requests.
3. For each tool call, the engine executes the corresponding connector action and collects the result.
4. The tool results are appended to the conversation and sent back to the LLM.
5. Steps 2-4 repeat until the LLM produces a final text response, or the configured round limit is reached.

**Safety limits:** The `max_tool_rounds` param (default: 10) caps the number of LLM-tool round trips. The `max_tool_calls_per_round` param (default: 10) caps how many tools the LLM can invoke in a single turn. If the round limit is exhausted, the engine makes one final call asking the LLM to respond with the information gathered so far.

**Error handling:** If a tool execution fails, the error message is sent back to the LLM as the tool result rather than crashing the workflow. This gives the LLM the opportunity to retry with different arguments or proceed without that tool's output.

See the [Tool Use Reference](/docs/workflow-reference/tools) for the tool schema and complete YAML examples.

## Triggers and Server Mode

Triggers introduce automatic execution. Up to this point, every concept on this page describes workflows triggered manually with `mantle run`.

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

The cron scheduler is built into the `mantle serve` process. It polls every 30 seconds, checks which cron triggers are due, and starts new executions for them.

### Webhook Triggers

A webhook trigger tells Mantle to start a new workflow execution when an HTTP POST arrives at a specific path. The request body is parsed as JSON and made available as `trigger.payload` in CEL expressions.

```yaml
triggers:
  - type: webhook
    path: "/hooks/on-deploy"
```

### Trigger Lifecycle

Triggers are managed through the same IaC lifecycle as the rest of the workflow definition. When you run `mantle apply`:

- Triggers defined in the YAML are registered (or updated) in the database
- Triggers that were previously registered but are no longer in the YAML are deregistered

The workflow YAML file is the single source of truth for trigger configuration.

### Cron vs Webhook: When to Use Which

| Use Case | Trigger Type | Why |
|---|---|---|
| Periodic data sync | Cron | Runs on a fixed schedule regardless of external events |
| Deploy notifications | Webhook | Fires in response to an external event (CI pipeline) |
| Daily report generation | Cron | Time-based, no external signal needed |
| GitHub push handler | Webhook | Event-driven, triggered by an external system |
| Scheduled cleanup | Cron | Maintenance task on a recurring schedule |

Many workflows benefit from both: a cron trigger for periodic runs and a webhook trigger for on-demand execution by external systems.
