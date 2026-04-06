# Design: Multi-Node Distribution (Phase 7)

> Phase 7 of Mantle V1.1. Enables multiple Mantle replicas to distribute workflow execution across nodes without duplication or loss.
>
> Designed for medium scale (5–20 replicas, hundreds of concurrent executions). Scaling considerations for 50+ replicas documented in Section 6.

## 1. Design Decisions

- **Postgres SKIP LOCKED only** — no external message broker. Preserves the "single binary + Postgres" architecture.
- **Hybrid orchestrator/worker model** — one node orchestrates each execution (determines ready steps), all nodes execute steps. Supports Phase 8's DAG resolution and tool-use sub-steps.
- **Claim and execute in separate transactions** — prevents holding row locks during step execution (which can take seconds to minutes).
- **Step output 1MB hard limit** — JSONB storage with enforced size cap. Users handle large payloads via the S3 connector. Threshold-based offload is a documented future upgrade path.

## 2. Schema Changes

### Additions to `step_executions`

```sql
ALTER TABLE step_executions ADD COLUMN claimed_by TEXT;
ALTER TABLE step_executions ADD COLUMN lease_expires_at TIMESTAMPTZ;
ALTER TABLE step_executions ADD COLUMN max_attempts INTEGER NOT NULL DEFAULT 1;
ALTER TABLE step_executions ADD COLUMN parent_step_id UUID REFERENCES step_executions(id);
ALTER TABLE step_executions ADD COLUMN cached_llm_responses JSONB DEFAULT '[]'::jsonb;

CREATE INDEX idx_step_executions_claimable
  ON step_executions (execution_id, status)
  WHERE status = 'pending';
```

- `claimed_by` — node identifier (`hostname:pid` or UUID), set when a worker claims a step.
- `lease_expires_at` — null when not leased. Workers must renew before expiry or the reaper reclaims the step.
- `max_attempts` — populated from the step's retry policy at row creation time. Denormalized here so the reaper can make retry decisions without loading the workflow definition.
- `parent_step_id` — null for top-level steps. Added now as a forward-looking schema addition so Phase 8 doesn't require an ALTER TABLE on a hot table. No Phase 7 code reads or writes this column.
- `cached_llm_responses` — used by Phase 8 for tool-use crash recovery. Added in the same migration to avoid future schema changes.

All columns added in a single goose migration (next sequential number after the latest existing migration). The migration includes the `execution_claims` table creation.

### New table: `execution_claims`

```sql
CREATE TABLE execution_claims (
    execution_id UUID PRIMARY KEY REFERENCES workflow_executions(id),
    claimed_by TEXT NOT NULL,
    lease_expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

Separates "who is orchestrating this execution" from "who is running this step." A node claims an execution to orchestrate it (determine which steps are ready, create pending rows). Any node can claim individual steps to execute.

No new tables for the work queue — `step_executions` with `status = 'pending'` IS the queue.

## 3. Claim/Execute Transaction Model

### Claim transaction (fast, milliseconds)

```sql
BEGIN;
SELECT id, step_name FROM step_executions
  WHERE execution_id = $1 AND status = 'pending'
  ORDER BY created_at
  LIMIT 1
  FOR UPDATE SKIP LOCKED;

UPDATE step_executions
  SET status = 'running', claimed_by = $2,
      lease_expires_at = NOW() + $3::interval,
      started_at = NOW()
  WHERE id = $4;
COMMIT;
```

### Execute (outside any transaction)

Run the connector, get the result.

### Complete transaction — success (fast)

```sql
UPDATE step_executions
  SET status = 'completed', output = $2,
      completed_at = NOW(), lease_expires_at = NULL
  WHERE id = $1 AND claimed_by = $3 AND status = 'running';
```

### Complete transaction — failure (fast)

```sql
UPDATE step_executions
  SET status = 'failed', error = $2,
      completed_at = NOW(), lease_expires_at = NULL
  WHERE id = $1 AND claimed_by = $3 AND status = 'running';
```

Both completion queries include `AND status = 'running'` to prevent a late-completing worker from overwriting a step that was already reclaimed and transitioned by the reaper. The `claimed_by` check acts as a **fencing token** — if the reaper reclaimed this step and another worker picked it up, the original worker's completion silently fails (0 rows updated). The worker detects this (0 rows affected) and discards its result.

**Fencing race note**: If a worker's lease expires but it completes work before another worker claims the step, the completion returns 0 rows (the reaper cleared `claimed_by`). This is safe — the step is back in `pending` and another worker will pick it up. No work is lost, though the original result is discarded.

### Lease renewal

For long-running steps, workers periodically extend their lease:

```sql
UPDATE step_executions
  SET lease_expires_at = NOW() + $2::interval
  WHERE id = $1 AND claimed_by = $3 AND status = 'running';
```

If this returns 0 rows, the lease was already reclaimed — the worker aborts execution via context cancellation.

## 4. Orchestrator Loop & Worker Loop

Each replica runs two goroutines concurrently.

### Orchestrator loop

Polls for unclaimed executions (new runs or orphaned orchestrations):

1. Claim an execution via `execution_claims` using `INSERT ... ON CONFLICT DO NOTHING` (only one node wins).
2. Load the workflow definition and completed steps.
3. Evaluate the step DAG — find steps whose dependencies are all satisfied.
4. INSERT pending `step_executions` rows for ready steps.
5. Wait for those steps to complete (poll `step_executions` for status changes).
6. Repeat steps 3–5 until all steps are done or a step fails.
7. Update `workflow_executions.status` to `completed` or `failed`.
8. Delete the `execution_claims` row.

The orchestrator renews its lease on `execution_claims` periodically. If it crashes, the reaper releases the claim, and another node's orchestrator picks it up — loading completed steps from the checkpoint and continuing.

**Poll interval**: 500ms for step completion checks. Jittered ±100ms.

### Worker loop

Polls for claimable steps across all executions:

1. `SELECT ... FOR UPDATE SKIP LOCKED` from `step_executions WHERE status = 'pending'`.
2. Claim, execute, complete (per Section 3's transaction model).
3. Loop.

**Poll interval**: 200ms base with exponential backoff up to 5s when no work is found. Resets to 200ms on successful claim. Jittered ±50ms to mitigate thundering herd.

### Thundering herd mitigation

Jitter plus SKIP LOCKED means even 20 replicas polling simultaneously get different rows — no contention. Backoff when idle keeps Postgres load low during quiet periods.

## 5. Reaper

A single goroutine per replica reclaims orphaned work. All replicas run the reaper — SKIP LOCKED prevents conflicts between them.

### Step reaper (runs every 30s)

**Retry model**: When the reaper reclaims a step, it marks the current row as `failed` and the orchestrator is responsible for creating a new `step_execution` row with `attempt + 1` (matching the existing unique constraint `(execution_id, step_name, attempt)`). This is consistent with the existing retry model where each attempt is a separate row.

Mark expired steps as failed:

```sql
UPDATE step_executions
  SET status = 'failed', error = 'lease expired',
      completed_at = NOW(), claimed_by = NULL, lease_expires_at = NULL
  WHERE status = 'running'
    AND lease_expires_at < NOW()
  RETURNING id, step_name, execution_id, attempt, max_attempts;
```

The orchestrator, on its next poll, checks for failed steps where `attempt < max_attempts` and creates a new row with `attempt + 1`. Steps where `attempt >= max_attempts` remain failed and trigger execution failure handling.

### Execution orchestrator reaper (runs every 30s, offset 15s from step reaper)

```sql
DELETE FROM execution_claims
  WHERE lease_expires_at < NOW()
  RETURNING execution_id;
```

Released executions are picked up by another node's orchestrator loop on its next poll.

### Lease durations

| Work type | Default lease | Renewal interval |
|---|---|---|
| Step execution | 60s | Every 20s |
| Execution orchestration | 120s | Every 40s |
| AI/LLM steps | 300s | Every 60s |

Lease duration is configurable per step type via `timeout` in workflow YAML — lease is set to `timeout + 30s` buffer. The defaults above apply when no timeout is specified.

### Reaper consistency

Multiple reapers running is safe — the UPDATE/DELETE statements are idempotent. If two reapers fire simultaneously, one gets the rows and the other gets 0 rows affected.

### Orchestrator failure handling

When a step fails (either from connector error or reaper expiry), the orchestrator:

1. Checks if `attempt < max_attempts` — if so, creates a new `step_execution` row with `attempt + 1` and `status = 'pending'`.
2. If retries are exhausted, marks the `workflow_execution` as `failed`. All pending steps for this execution are set to `cancelled` (not executed).
3. In-flight steps for the same execution are allowed to complete (their results are checkpointed but the execution is already marked failed).

### Timestamp discipline

All lease timestamps use database server time via `NOW()`, never application-level timestamps. This eliminates clock skew issues between replicas and the database. Workers and reapers never compute lease expiry using local clocks.

## 6. Scaling Considerations

### Medium scale (5–20 replicas) — design target

- SKIP LOCKED contention is negligible at this scale.
- Connection count: ~4 connections per replica (orchestrator, worker, reaper, lease renewal). 20 replicas = 80 connections, within Postgres defaults.
- Partial index on `status = 'pending'` keeps claim queries fast regardless of historical row count.

### Large scale inflection points (50+ replicas)

**Connection pooling**: At 50+ replicas (150+ connections), deploy PgBouncer in transaction mode. The claim/execute/complete pattern uses short transactions, so this works out of the box.

**Poll frequency tuning**: 50+ workers polling at 200ms = 250 queries/second when idle. Exponential backoff to 5s reduces this to ~10 queries/second at idle. If bursts are a concern, `LISTEN/NOTIFY` can replace polling — the orchestrator does `NOTIFY step_ready` after inserting pending rows, workers `LISTEN` instead of polling. Noted as a future optimization.

**Completed row archival**: Add a `completed_before` archival job that moves rows older than a configurable retention period (default 30 days) to `step_executions_archive`. The partial index on `status = 'pending'` means query performance is unaffected by table size, but storage and vacuum costs grow.

**Step output size limit**: 1MB hard limit enforced at the engine level before writing to JSONB. Steps exceeding this fail with a clear error suggesting the S3 connector pattern. Threshold-based offload to object storage is the natural upgrade path.

## 7. Observability

### New Prometheus metrics

- `mantle_queue_depth` (gauge) — count of pending steps
- `mantle_claim_duration_seconds` (histogram) — time from pending to claimed
- `mantle_lease_renewals_total` (counter)
- `mantle_lease_expirations_total` (counter) — indicates node failures or slow steps
- `mantle_reaper_reclaimed_total` (counter)

## 8. Testing Strategy

Concurrency tests are required to prove correctness under load with deliberate crashes.

### Test harness

Integration tests using testcontainers (real Postgres). Multiple worker goroutines within a single test process simulate multi-node behavior — each goroutine gets its own `node_id` and runs independent orchestrator/worker/reaper loops.

### Invariants to assert

- **No step lost**: every step in every workflow execution reaches a terminal state (`completed`, `failed`, `skipped`, `cancelled`).
- **No step duplicated**: for a given `(execution_id, step_name, attempt)`, at most one row has `status = 'completed'` with non-null output.
- **All executions complete**: every workflow execution reaches `completed` or `failed` within a timeout.
- **Fencing correctness**: a worker whose lease expired cannot overwrite a step completed by another worker.

### Crash simulation tests

1. **Worker crash mid-execution**: Kill a worker goroutine (cancel its context) while a step is `running`. Verify: reaper reclaims the step within `lease_timeout + reaper_interval`, another worker picks it up, step completes.
2. **Orchestrator crash**: Kill the orchestrator goroutine for an execution. Verify: reaper releases the `execution_claims` row, another orchestrator picks up the execution and continues from checkpoint.
3. **Multiple simultaneous crashes**: Kill 2 of 3 workers. Verify: remaining worker completes all work, no steps lost.
4. **Reaper under load**: Run 10 workflows with 5 workers, kill 3 workers mid-flight. Verify: all workflows eventually complete with the remaining 2 workers.

### Load tests

- 3+ worker goroutines, 50 concurrent workflow executions, each with 5-10 steps.
- Assert all invariants hold after completion.
- Measure: claim latency, queue depth over time, total throughput.

## 9. Configuration

New `mantle.yaml` keys under `engine:`:

```yaml
engine:
  node_id: ""                        # auto-generated if empty (hostname:pid)
  worker_poll_interval: 200ms
  worker_max_backoff: 5s
  orchestrator_poll_interval: 500ms
  step_lease_duration: 60s
  orchestration_lease_duration: 120s
  reaper_interval: 30s
  step_output_max_bytes: 1048576     # 1MB
```
