# Design: AI Tool Use & Parallel Execution (Phase 8)

> Phase 8 of Mantle V1.1. Enables LLM steps to call tools during execution, and independent workflow steps to run in parallel.
>
> Builds on Phase 7's multi-node distribution primitives: orchestrator/worker model, SKIP LOCKED claiming, lease/reaper lifecycle, and sub-step rows via `parent_step_id`. See [Multi-Node Distribution Design](2026-03-19-multi-node-distribution-design.md) for those foundations.

## 1. Design Decisions

- **Implicit parallelism via `depends_on`** — steps declare dependencies, the orchestrator resolves the DAG and runs independent steps concurrently. No explicit parallel block syntax.
- **Sub-steps as first-class `step_executions` rows** — tool call results stored as child rows with `parent_step_id`. Free checkpointing, free multi-node distribution, and visible in `mantle logs`.
- **Full LLM response + tool result caching for crash recovery** — deterministic replay on crash. Worst case: one redundant LLM call.
- **Single-level tool use** — tools execute connectors, they cannot themselves declare tools. Data model supports future multi-level (child rows can have children) without rearchitecting.

## 2. Parallel Execution via DAG Resolution

### Workflow YAML syntax

```yaml
steps:
  - name: fetch_users
    action: http/request
    params:
      url: "https://api.example.com/users"

  - name: fetch_orders
    action: http/request
    params:
      url: "https://api.example.com/orders"

  - name: summarize
    action: ai/completion
    depends_on: [fetch_users, fetch_orders]
    params:
      prompt: "Summarize: {{ steps.fetch_users.output.body }} and {{ steps.fetch_orders.output.body }}"
```

`fetch_users` and `fetch_orders` have no dependencies — the orchestrator creates both as `pending` simultaneously, and workers execute them in parallel. `summarize` waits until both are `completed`.

### Orchestrator DAG logic

The orchestrator from Phase 7 changes minimally. Instead of creating steps sequentially, it:

1. Builds an in-memory DAG from `depends_on` declarations.
2. On each iteration, finds steps where all dependencies are in `completed` or `skipped` status.
3. Creates `pending` rows for all ready steps at once.
4. Waits for any status change, then re-evaluates.

### Implicit dependency detection

If a step's CEL expressions reference `steps.foo.output`, `foo` is added as an implicit dependency even without an explicit `depends_on`. This is validated at `mantle validate` time — the validator parses CEL expressions and extracts step references.

### Validation rules

- `mantle validate` checks for cycles in the dependency graph.
- References to undefined step names are errors.
- `depends_on` must be an array of step names.
- Conditional steps (`if:`) can be depended on — if skipped, dependents still proceed (skipped counts as "resolved").

## 3. Tool Use Execution Model

### Workflow YAML syntax

```yaml
steps:
  - name: research_agent
    action: ai/completion
    params:
      model: gpt-4o
      system_prompt: "You are a research assistant."
      prompt: "Find the current weather in {{ inputs.city }}"
      tools:
        - name: get_weather
          action: http/request
          params:
            url: "https://api.weather.com/v1/current?city={{ tool_input.city }}"
        - name: get_forecast
          action: http/request
          params:
            url: "https://api.weather.com/v1/forecast?city={{ tool_input.city }}"
      max_tool_rounds: 10
```

Tools are declared inline on the AI step. Each tool maps to a connector action. `tool_input` is a CEL variable populated from the LLM's tool call arguments. `max_tool_rounds` limits the number of LLM↔tool round-trips (default: 10).

### Execution flow

The AI connector drives a loop:

1. **Initial LLM call** — send prompt + tool definitions to the LLM.
2. **LLM responds** — either with final text (done) or with tool call requests.
3. **Tool execution** — for each tool call, the AI connector creates a child `step_execution` row with `parent_step_id` pointing to the AI step. These go through the normal claim/execute/complete flow from Phase 7.
4. **Tool results returned to LLM** — the AI connector collects completed tool results, appends them to the conversation, and calls the LLM again.
5. **Repeat** until the LLM returns a final response or `max_tool_rounds` is exhausted.

### Sub-step naming and schema

Child step executions use the `parent_step_id` column from Phase 7:

```
step_executions:
  id: uuid-child-1
  execution_id: <same as parent>
  parent_step_id: uuid-parent-ai-step
  step_name: "research_agent/tool/get_weather/0"   -- parent/tool/name/round
  status: pending → running → completed
  output: { tool result }
```

The naming convention `parent/tool/name/round` gives unique identification and readable `mantle logs` output.

### Tool loop orchestration

The **worker that claimed the AI step** drives the tool-use loop — not the execution-level orchestrator. The AI connector acts as a mini-orchestrator for its sub-steps:

1. Creates child `step_execution` rows with `status = 'pending'`.
2. Waits for them to be claimed and completed (by any worker, including itself).
3. Collects results and continues the LLM conversation.

The parent AI step's lease is renewed throughout this process. If the worker crashes, the reaper reclaims the parent step, and recovery kicks in (Section 4).

## 4. Crash Recovery & Tool Call Caching

LLMs are non-deterministic — replaying a crashed AI step won't produce the same tool calls. Recovery must be deterministic.

### What gets cached

Every LLM response and every tool result is persisted before acting on it.

**LLM response caching**: After each LLM call, the full response (including tool call requests) is appended to a JSONB array on the parent AI step:

```sql
UPDATE step_executions
  SET cached_llm_responses = cached_llm_responses || $2::jsonb
  WHERE id = $1;
```

**Tool result caching**: Already handled — each tool call is a child `step_execution` row with its output persisted on completion.

### Schema addition

```sql
ALTER TABLE step_executions ADD COLUMN cached_llm_responses JSONB DEFAULT '[]'::jsonb;
```

### Recovery flow

When a crashed AI step is reclaimed and restarted:

1. Load `cached_llm_responses` from the parent row.
2. Load all child `step_execution` rows and their statuses/outputs.
3. Reconstruct the conversation state:
   - For each cached round: the LLM response is known, the tool results are known (from child rows).
   - If all tool calls in the last round are `completed`: replay from cache, continue to next round.
   - If some tool calls in the last round are `pending` or `running`: wait for them (they're already in the system).
   - If the crash happened before tool call rows were created: create them from the cached LLM response.
   - If the crash happened before the LLM response was cached: re-call the LLM (this is the only case where work is redone — one LLM call).

### Recovery scenarios

| Crash point | What's cached | Recovery action |
|---|---|---|
| After LLM response cached, before tool rows created | LLM response | Create tool rows from cached response |
| After tool rows created, tools still running | LLM response + partial tool results | Wait for in-flight tools, create missing ones |
| After all tools complete, before next LLM call | LLM response + all tool results | Call LLM with full conversation history |
| Before LLM response cached | Previous rounds only | Re-call LLM for current round |

### Idempotency of tool row creation

Tool child rows use `INSERT ... ON CONFLICT DO NOTHING` keyed on `(execution_id, step_name, attempt)`. The step name `research_agent/tool/get_weather/0` is deterministic from the cached LLM response, so re-creating them after a crash doesn't produce duplicates.

### Cost of recovery

Worst case: one redundant LLM call (the one that wasn't cached). All tool executions are either reused from cache or already in-flight.

## 5. Recursion Limits, Validation & Observability

### Recursion limits

Single-level tool use for V1.1. The data model supports future multi-level (child rows can have children), but the AI connector enforces:

- `max_tool_rounds` — max LLM↔tool round-trips per AI step (default 10, max 50).
- `max_tool_calls_per_round` — max tool calls the LLM can make in a single response (default 10, max 25).
- Total tool executions per AI step capped at `max_tool_rounds × max_tool_calls_per_round` (default 100, max 1250).

When any limit is hit, the AI connector sends a final message to the LLM: "Tool use limit reached. Provide your best response with available information." If the LLM still returns tool calls, the step fails with a clear error.

### Validation rules

`mantle validate` checks:

- Tool `name` is unique within a step's tool list.
- Tool `action` references a valid connector.
- Tool params are valid for that connector (same validation as regular steps).
- `max_tool_rounds` is within bounds.
- `depends_on` cycle detection includes implicit dependencies from CEL expressions.
- Circular tool references are impossible in V1.1 (tools can't declare sub-tools).

### `mantle logs` output

```
EXECUTION abc123
  ✓ fetch_data          completed  1.2s
  ✓ research_agent      completed  8.4s
    ├─ round 1
    │  ├─ get_weather    completed  0.8s
    │  └─ get_forecast   completed  1.1s
    ├─ round 2
    │  └─ get_weather    completed  0.6s
    └─ final response    "The weather in..."
  ✓ send_report         completed  0.3s
```

Sub-steps are indented under their parent with round grouping.

### New Prometheus metrics

- `mantle_tool_calls_total` (counter, labels: `step`, `tool`, `status`)
- `mantle_tool_rounds_total` (counter, labels: `step`)
- `mantle_tool_round_duration_seconds` (histogram)
- `mantle_llm_cache_hits_total` (counter) — recovery replays from cache
- `mantle_parallel_steps_in_flight` (gauge) — concurrent step executions per workflow

## 6. Configuration

New `mantle.yaml` keys:

```yaml
engine:
  default_max_tool_rounds: 10
  default_max_tool_calls_per_round: 10
  ai_step_lease_duration: 300s       # longer lease for tool-use loops
```
