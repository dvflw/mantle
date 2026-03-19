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
ALTER TABLE step_executions ADD COLUMN parent_step_id UUID REFERENCES step_executions(id);

CREATE INDEX idx_step_executions_claimable
  ON step_executions (execution_id, status)
  WHERE status = 'pending';
```

- `claimed_by` — node identifier (`hostname:pid` or UUID), set when a worker claims a step.
- `lease_expires_at` — null when not leased. Workers must renew before expiry or the reaper reclaims the step.
- `parent_step_id` — null for top-level steps. Used by Phase 8 for tool-use sub-steps.

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

### Complete transaction (fast)

```sql
UPDATE step_executions
  SET status = 'completed', output = $2,
      completed_at = NOW(), lease_expires_at = NULL
  WHERE id = $1 AND claimed_by = $3;
```

The `claimed_by` check on completion acts as a **fencing token**. If the reaper reclaimed this step and another worker picked it up, the original worker's completion silently fails (0 rows updated). The worker detects this and discards its result.

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

Reset steps that can be retried:

```sql
UPDATE step_executions
  SET status = 'pending', claimed_by = NULL, lease_expires_at = NULL
  WHERE status = 'running'
    AND lease_expires_at < NOW()
    AND attempt < max_attempts
  RETURNING id, step_name, execution_id;
```

Fail steps that have exhausted retries:

```sql
UPDATE step_executions
  SET status = 'failed', error = 'lease expired, retries exhausted',
      completed_at = NOW(), lease_expires_at = NULL
  WHERE status = 'running'
    AND lease_expires_at < NOW()
    AND attempt >= max_attempts;
```

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

## 6. Scaling Considerations

### Medium scale (5–20 replicas) — design target

- SKIP LOCKED contention is negligible at this scale.
- Connection count: ~3 connections per replica (orchestrator, worker, reaper). 20 replicas = 60 connections, within Postgres defaults.
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

## 8. Configuration

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
