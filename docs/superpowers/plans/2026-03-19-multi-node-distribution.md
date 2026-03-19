# Multi-Node Distribution (Phase 7) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable multiple Mantle replicas to distribute workflow step execution across nodes using Postgres SKIP LOCKED, with no step duplication or loss.

**Architecture:** Hybrid orchestrator/worker model. Each replica runs an orchestrator loop (claims executions, creates pending steps), a worker loop (claims and executes individual steps via SKIP LOCKED), and a reaper (reclaims orphaned work via lease expiry). Claim and execute are separate transactions with fencing tokens.

**Tech Stack:** Go, Postgres (SKIP LOCKED, FOR UPDATE), testcontainers for integration tests, Prometheus for observability.

**Spec:** `docs/superpowers/specs/2026-03-19-multi-node-distribution-design.md`

---

## File Structure

### New Files
- `internal/db/migrations/007_multi_node.sql` — schema changes (columns, indexes, execution_claims table)
- `internal/engine/claim.go` — step claiming, completion, and lease renewal logic
- `internal/engine/orchestrator.go` — orchestrator loop (claim execution, create pending steps, wait for completion)
- `internal/engine/worker.go` — worker loop (poll for pending steps, claim, execute, complete)
- `internal/engine/reaper.go` — reaper goroutine (reclaim expired leases)
- `internal/engine/claim_test.go` — unit tests for claim/complete/renew
- `internal/engine/orchestrator_test.go` — orchestrator tests
- `internal/engine/worker_test.go` — worker tests
- `internal/engine/reaper_test.go` — reaper tests
- `internal/engine/distributed_test.go` — multi-node concurrency integration tests

### Modified Files
- `internal/config/config.go` — add EngineConfig struct with node_id, poll intervals, lease durations
- `internal/engine/engine.go` — refactor to support distributed mode; extract step execution logic
- `internal/server/server.go` — start orchestrator/worker/reaper goroutines in serve mode
- `internal/metrics/metrics.go` — add queue depth, claim duration, lease, reaper metrics
- `internal/workflow/workflow.go` — add `DependsOn` field to Step struct

---

## Task 1: Database Migration

**Files:**
- Create: `internal/db/migrations/007_multi_node.sql`

- [ ] **Step 1: Write the migration file**

```sql
-- +goose Up

-- Multi-node distribution columns on step_executions
ALTER TABLE step_executions ADD COLUMN claimed_by TEXT;
ALTER TABLE step_executions ADD COLUMN lease_expires_at TIMESTAMPTZ;
ALTER TABLE step_executions ADD COLUMN max_attempts INTEGER NOT NULL DEFAULT 1;
ALTER TABLE step_executions ADD COLUMN parent_step_id UUID REFERENCES step_executions(id);
ALTER TABLE step_executions ADD COLUMN cached_llm_responses JSONB DEFAULT '[]'::jsonb;

-- Partial index for efficient SKIP LOCKED queries on pending steps
CREATE INDEX idx_step_executions_claimable
  ON step_executions (execution_id, status)
  WHERE status = 'pending';

-- Index for reaper queries on expired leases
CREATE INDEX idx_step_executions_lease_expiry
  ON step_executions (lease_expires_at)
  WHERE status = 'running' AND lease_expires_at IS NOT NULL;

-- Execution orchestration claims
CREATE TABLE execution_claims (
    execution_id UUID PRIMARY KEY REFERENCES workflow_executions(id),
    claimed_by TEXT NOT NULL,
    lease_expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS execution_claims;
DROP INDEX IF EXISTS idx_step_executions_lease_expiry;
DROP INDEX IF EXISTS idx_step_executions_claimable;
ALTER TABLE step_executions DROP COLUMN IF EXISTS cached_llm_responses;
ALTER TABLE step_executions DROP COLUMN IF EXISTS parent_step_id;
ALTER TABLE step_executions DROP COLUMN IF EXISTS max_attempts;
ALTER TABLE step_executions DROP COLUMN IF EXISTS lease_expires_at;
ALTER TABLE step_executions DROP COLUMN IF EXISTS claimed_by;
```

- [ ] **Step 2: Run migration against local Postgres**

Run: `docker-compose up -d && make migrate`
Expected: Migration applies cleanly, no errors.

- [ ] **Step 3: Verify schema**

Run: `docker-compose exec postgres psql -U mantle -d mantle -c "\d step_executions"` and `docker-compose exec postgres psql -U mantle -d mantle -c "\d execution_claims"`
Expected: New columns and table visible.

- [ ] **Step 4: Commit**

```bash
git add internal/db/migrations/007_multi_node.sql
git commit -m "feat(phase7): add multi-node distribution schema migration"
```

---

## Task 2: Engine Configuration

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/config/config_test.go`:

```go
func TestConfig_EngineDefaults(t *testing.T) {
	cmd := newTestCommand()
	cfg, err := Load(cmd)
	require.NoError(t, err)

	assert.Equal(t, 200*time.Millisecond, cfg.Engine.WorkerPollInterval)
	assert.Equal(t, 5*time.Second, cfg.Engine.WorkerMaxBackoff)
	assert.Equal(t, 500*time.Millisecond, cfg.Engine.OrchestratorPollInterval)
	assert.Equal(t, 60*time.Second, cfg.Engine.StepLeaseDuration)
	assert.Equal(t, 120*time.Second, cfg.Engine.OrchestrationLeaseDuration)
	assert.Equal(t, 300*time.Second, cfg.Engine.AIStepLeaseDuration)
	assert.Equal(t, 30*time.Second, cfg.Engine.ReaperInterval)
	assert.Equal(t, 1048576, cfg.Engine.StepOutputMaxBytes)
	assert.NotEmpty(t, cfg.Engine.NodeID)
}
```

Note: `newTestCommand()` is an existing test helper in `config_test.go` that creates a cobra.Command with all config flags registered. The `Load` function takes a `*cobra.Command`, not a string.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestConfig_EngineDefaults -v`
Expected: FAIL — `cfg.Engine` doesn't exist yet.

- [ ] **Step 3: Add EngineConfig struct and defaults**

In `internal/config/config.go`, add:

```go
type EngineConfig struct {
	NodeID                    string        `mapstructure:"node_id"`
	WorkerPollInterval        time.Duration `mapstructure:"worker_poll_interval"`
	WorkerMaxBackoff          time.Duration `mapstructure:"worker_max_backoff"`
	OrchestratorPollInterval  time.Duration `mapstructure:"orchestrator_poll_interval"`
	StepLeaseDuration         time.Duration `mapstructure:"step_lease_duration"`
	OrchestrationLeaseDuration time.Duration `mapstructure:"orchestration_lease_duration"`
	AIStepLeaseDuration       time.Duration `mapstructure:"ai_step_lease_duration"`
	ReaperInterval            time.Duration `mapstructure:"reaper_interval"`
	StepOutputMaxBytes        int           `mapstructure:"step_output_max_bytes"`
}
```

Add `Engine EngineConfig `mapstructure:"engine"`` to the Config struct.

In the `Load` function, add defaults:

```go
viper.SetDefault("engine.worker_poll_interval", 200*time.Millisecond)
viper.SetDefault("engine.worker_max_backoff", 5*time.Second)
viper.SetDefault("engine.orchestrator_poll_interval", 500*time.Millisecond)
viper.SetDefault("engine.step_lease_duration", 60*time.Second)
viper.SetDefault("engine.orchestration_lease_duration", 120*time.Second)
viper.SetDefault("engine.ai_step_lease_duration", 300*time.Second)
viper.SetDefault("engine.reaper_interval", 30*time.Second)
viper.SetDefault("engine.step_output_max_bytes", 1048576)
```

Add env var bindings (matching existing pattern in the `Load` function):

```go
viper.BindEnv("engine.node_id", "MANTLE_ENGINE_NODE_ID")
viper.BindEnv("engine.worker_poll_interval", "MANTLE_ENGINE_WORKER_POLL_INTERVAL")
viper.BindEnv("engine.worker_max_backoff", "MANTLE_ENGINE_WORKER_MAX_BACKOFF")
viper.BindEnv("engine.orchestrator_poll_interval", "MANTLE_ENGINE_ORCHESTRATOR_POLL_INTERVAL")
viper.BindEnv("engine.step_lease_duration", "MANTLE_ENGINE_STEP_LEASE_DURATION")
viper.BindEnv("engine.orchestration_lease_duration", "MANTLE_ENGINE_ORCHESTRATION_LEASE_DURATION")
viper.BindEnv("engine.ai_step_lease_duration", "MANTLE_ENGINE_AI_STEP_LEASE_DURATION")
viper.BindEnv("engine.reaper_interval", "MANTLE_ENGINE_REAPER_INTERVAL")
viper.BindEnv("engine.step_output_max_bytes", "MANTLE_ENGINE_STEP_OUTPUT_MAX_BYTES")
```

Generate a default NodeID if empty (after Viper unmarshals):

```go
if cfg.Engine.NodeID == "" {
	hostname, _ := os.Hostname()
	cfg.Engine.NodeID = fmt.Sprintf("%s:%d", hostname, os.Getpid())
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -run TestConfig_EngineDefaults -v`
Expected: PASS

- [ ] **Step 5: Run full config test suite**

Run: `go test ./internal/config/ -v`
Expected: All tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(phase7): add engine configuration for multi-node distribution"
```

---

## Task 3: Step Claiming and Completion Logic

**Files:**
- Create: `internal/engine/claim.go`
- Create: `internal/engine/claim_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/engine/claim_test.go`:

```go
package engine

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaimStep_ClaimsPendingStep(t *testing.T) {
	db := setupTestDB(t)
	c := &Claimer{DB: db, NodeID: "node-1", LeaseDuration: 60 * time.Second}

	execID := createTestExecution(t, db)
	insertPendingStep(t, db, execID, "step-a", 1)

	claim, err := c.ClaimStep(context.Background(), execID)
	require.NoError(t, err)
	require.NotNil(t, claim)
	assert.Equal(t, "step-a", claim.StepName)
	assert.Equal(t, "node-1", claim.ClaimedBy)
}

func TestClaimStep_SkipsLockedRows(t *testing.T) {
	db := setupTestDB(t)
	c1 := &Claimer{DB: db, NodeID: "node-1", LeaseDuration: 60 * time.Second}
	c2 := &Claimer{DB: db, NodeID: "node-2", LeaseDuration: 60 * time.Second}

	execID := createTestExecution(t, db)
	insertPendingStep(t, db, execID, "step-a", 1)

	// Node 1 claims the only step
	claim1, err := c1.ClaimStep(context.Background(), execID)
	require.NoError(t, err)
	require.NotNil(t, claim1)

	// Node 2 finds nothing
	claim2, err := c2.ClaimStep(context.Background(), execID)
	require.NoError(t, err)
	assert.Nil(t, claim2)
}

func TestCompleteStep_Success(t *testing.T) {
	db := setupTestDB(t)
	c := &Claimer{DB: db, NodeID: "node-1", LeaseDuration: 60 * time.Second}

	execID := createTestExecution(t, db)
	insertPendingStep(t, db, execID, "step-a", 1)

	claim, err := c.ClaimStep(context.Background(), execID)
	require.NoError(t, err)

	output := map[string]any{"result": "ok"}
	ok, err := c.CompleteStep(context.Background(), claim.ID, output, nil)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestCompleteStep_FencingRejectsStaleWorker(t *testing.T) {
	db := setupTestDB(t)
	c1 := &Claimer{DB: db, NodeID: "node-1", LeaseDuration: 60 * time.Second}
	c2 := &Claimer{DB: db, NodeID: "node-2", LeaseDuration: 60 * time.Second}

	execID := createTestExecution(t, db)
	insertPendingStep(t, db, execID, "step-a", 1)

	// Node 1 claims
	claim, _ := c1.ClaimStep(context.Background(), execID)

	// Simulate reaper: reset to pending, node 2 claims
	resetStepToPending(t, db, claim.ID)
	claim2, _ := c2.ClaimStep(context.Background(), execID)
	require.NotNil(t, claim2)

	// Node 1 tries to complete — rejected by fencing
	ok, err := c1.CompleteStep(context.Background(), claim.ID, map[string]any{}, nil)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestClaimAnyStep_ClaimsAcrossExecutions(t *testing.T) {
	db := setupTestDB(t)
	c := &Claimer{DB: db, NodeID: "node-1", LeaseDuration: 60 * time.Second}

	exec1 := createTestExecution(t, db)
	exec2 := createTestExecution(t, db)
	insertPendingStep(t, db, exec1, "step-a", 1)
	insertPendingStep(t, db, exec2, "step-b", 1)

	claim1, execID1, err := c.ClaimAnyStep(context.Background())
	require.NoError(t, err)
	require.NotNil(t, claim1)
	assert.NotEmpty(t, execID1)

	claim2, execID2, err := c.ClaimAnyStep(context.Background())
	require.NoError(t, err)
	require.NotNil(t, claim2)
	assert.NotEqual(t, claim1.ID, claim2.ID)

	claim3, _, err := c.ClaimAnyStep(context.Background())
	require.NoError(t, err)
	assert.Nil(t, claim3) // No more pending steps
}

func TestRenewLease_ExtendsExpiry(t *testing.T) {
	db := setupTestDB(t)
	c := &Claimer{DB: db, NodeID: "node-1", LeaseDuration: 60 * time.Second}

	execID := createTestExecution(t, db)
	insertPendingStep(t, db, execID, "step-a", 1)

	claim, _ := c.ClaimStep(context.Background(), execID)

	ok, err := c.RenewLease(context.Background(), claim.ID)
	require.NoError(t, err)
	assert.True(t, ok)
}
```

Note: `setupTestDB`, `createTestExecution`, `insertPendingStep`, `resetStepToPending` are test helpers. Reuse the existing `setupTestDB` from `engine_test.go` and add the new helpers.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/engine/ -run TestClaim -v`
Expected: FAIL — `Claimer` type doesn't exist.

- [ ] **Step 3: Write the Claimer implementation**

Create `internal/engine/claim.go`:

```go
package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// StepClaim represents a claimed step ready for execution.
type StepClaim struct {
	ID        string
	StepName  string
	Attempt   int
	ClaimedBy string
}

// Claimer handles step claiming, completion, and lease renewal using Postgres SKIP LOCKED.
type Claimer struct {
	DB            *sql.DB
	NodeID        string
	LeaseDuration time.Duration
}

// ClaimStep attempts to claim a pending step for the given execution.
// Returns nil if no step is available (all claimed or none pending).
func (c *Claimer) ClaimStep(ctx context.Context, executionID string) (*StepClaim, error) {
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin claim tx: %w", err)
	}
	defer tx.Rollback()

	var id, stepName string
	var attempt int
	err = tx.QueryRowContext(ctx, `
		SELECT id, step_name, attempt FROM step_executions
		WHERE execution_id = $1 AND status = 'pending'
		ORDER BY created_at
		LIMIT 1
		FOR UPDATE SKIP LOCKED
	`, executionID).Scan(&id, &stepName, &attempt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("select pending step: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE step_executions
		SET status = 'running', claimed_by = $2,
		    lease_expires_at = NOW() + $3::interval,
		    started_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, id, c.NodeID, fmt.Sprintf("%d seconds", int(c.LeaseDuration.Seconds())))
	if err != nil {
		return nil, fmt.Errorf("update step to running: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit claim tx: %w", err)
	}

	return &StepClaim{
		ID:        id,
		StepName:  stepName,
		Attempt:   attempt,
		ClaimedBy: c.NodeID,
	}, nil
}

// ClaimAnyStep attempts to claim any pending step across all executions.
func (c *Claimer) ClaimAnyStep(ctx context.Context) (*StepClaim, string, error) {
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, "", fmt.Errorf("begin claim tx: %w", err)
	}
	defer tx.Rollback()

	var id, stepName, executionID string
	var attempt int
	err = tx.QueryRowContext(ctx, `
		SELECT id, execution_id, step_name, attempt FROM step_executions
		WHERE status = 'pending' AND parent_step_id IS NULL
		ORDER BY created_at
		LIMIT 1
		FOR UPDATE SKIP LOCKED
	`, ).Scan(&id, &executionID, &stepName, &attempt)

	if err == sql.ErrNoRows {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("select pending step: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE step_executions
		SET status = 'running', claimed_by = $2,
		    lease_expires_at = NOW() + $3::interval,
		    started_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, id, c.NodeID, fmt.Sprintf("%d seconds", int(c.LeaseDuration.Seconds())))
	if err != nil {
		return nil, "", fmt.Errorf("update step to running: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, "", fmt.Errorf("commit claim tx: %w", err)
	}

	return &StepClaim{
		ID:        id,
		StepName:  stepName,
		Attempt:   attempt,
		ClaimedBy: c.NodeID,
	}, executionID, nil
}

// CompleteStep marks a claimed step as completed or failed.
// Returns false if the fencing check fails (step was reclaimed by reaper).
func (c *Claimer) CompleteStep(ctx context.Context, stepID string, output map[string]any, stepErr error) (bool, error) {
	if stepErr != nil {
		return c.failStep(ctx, stepID, stepErr.Error())
	}

	outputJSON, err := json.Marshal(output)
	if err != nil {
		return false, fmt.Errorf("marshal output: %w", err)
	}

	result, err := c.DB.ExecContext(ctx, `
		UPDATE step_executions
		SET status = 'completed', output = $2,
		    completed_at = NOW(), lease_expires_at = NULL, updated_at = NOW()
		WHERE id = $1 AND claimed_by = $3 AND status = 'running'
	`, stepID, outputJSON, c.NodeID)
	if err != nil {
		return false, fmt.Errorf("complete step: %w", err)
	}

	rows, _ := result.RowsAffected()
	return rows > 0, nil
}

func (c *Claimer) failStep(ctx context.Context, stepID string, errMsg string) (bool, error) {
	result, err := c.DB.ExecContext(ctx, `
		UPDATE step_executions
		SET status = 'failed', error = $2,
		    completed_at = NOW(), lease_expires_at = NULL, updated_at = NOW()
		WHERE id = $1 AND claimed_by = $3 AND status = 'running'
	`, stepID, errMsg, c.NodeID)
	if err != nil {
		return false, fmt.Errorf("fail step: %w", err)
	}

	rows, _ := result.RowsAffected()
	return rows > 0, nil
}

// RenewLease extends the lease on a claimed step.
// Returns false if the lease was already reclaimed.
func (c *Claimer) RenewLease(ctx context.Context, stepID string) (bool, error) {
	result, err := c.DB.ExecContext(ctx, `
		UPDATE step_executions
		SET lease_expires_at = NOW() + $2::interval, updated_at = NOW()
		WHERE id = $1 AND claimed_by = $3 AND status = 'running'
	`, stepID, fmt.Sprintf("%d seconds", int(c.LeaseDuration.Seconds())), c.NodeID)
	if err != nil {
		return false, fmt.Errorf("renew lease: %w", err)
	}

	rows, _ := result.RowsAffected()
	return rows > 0, nil
}
```

- [ ] **Step 4: Add test helpers**

Add to `internal/engine/claim_test.go` (or a shared test helpers file):

```go
func createTestExecution(t *testing.T, db *sql.DB) string {
	t.Helper()
	var id string
	err := db.QueryRow(`
		INSERT INTO workflow_executions (workflow_name, workflow_version, status, inputs)
		VALUES ('test-workflow', 1, 'running', '{}')
		RETURNING id
	`).Scan(&id)
	require.NoError(t, err)
	return id
}

func insertPendingStep(t *testing.T, db *sql.DB, execID, stepName string, attempt int) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO step_executions (execution_id, step_name, attempt, status, max_attempts)
		VALUES ($1, $2, $3, 'pending', 3)
	`, execID, stepName, attempt)
	require.NoError(t, err)
}

func resetStepToPending(t *testing.T, db *sql.DB, stepID string) {
	t.Helper()
	_, err := db.Exec(`
		UPDATE step_executions
		SET status = 'pending', claimed_by = NULL, lease_expires_at = NULL
		WHERE id = $1
	`, stepID)
	require.NoError(t, err)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/engine/ -run TestClaim -v`
Expected: All claim tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/engine/claim.go internal/engine/claim_test.go
git commit -m "feat(phase7): add step claiming with SKIP LOCKED and fencing tokens"
```

---

## Task 4: Reaper

**Files:**
- Create: `internal/engine/reaper.go`
- Create: `internal/engine/reaper_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/engine/reaper_test.go`:

```go
package engine

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReaper_ReclaimsExpiredSteps(t *testing.T) {
	db := setupTestDB(t)
	r := &Reaper{DB: db}

	execID := createTestExecution(t, db)
	insertPendingStep(t, db, execID, "step-a", 1)

	// Claim the step with an already-expired lease
	_, err := db.Exec(`
		UPDATE step_executions
		SET status = 'running', claimed_by = 'dead-node',
		    lease_expires_at = NOW() - INTERVAL '1 second',
		    started_at = NOW(), max_attempts = 3
		WHERE execution_id = $1 AND step_name = 'step-a'
	`, execID)
	require.NoError(t, err)

	reclaimed, err := r.ReapSteps(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, reclaimed)

	// Verify step is now failed (not pending — orchestrator handles retry)
	var status string
	err = db.QueryRow(`SELECT status FROM step_executions WHERE execution_id = $1 AND step_name = 'step-a'`, execID).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "failed", status)
}

func TestReaper_IgnoresActiveLeases(t *testing.T) {
	db := setupTestDB(t)
	r := &Reaper{DB: db}

	execID := createTestExecution(t, db)
	insertPendingStep(t, db, execID, "step-a", 1)

	// Claim with a future lease
	_, err := db.Exec(`
		UPDATE step_executions
		SET status = 'running', claimed_by = 'active-node',
		    lease_expires_at = NOW() + INTERVAL '60 seconds',
		    started_at = NOW()
		WHERE execution_id = $1 AND step_name = 'step-a'
	`, execID)
	require.NoError(t, err)

	reclaimed, err := r.ReapSteps(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, reclaimed)
}

func TestReaper_ReclainsExpiredExecutionClaims(t *testing.T) {
	db := setupTestDB(t)
	r := &Reaper{DB: db}

	execID := createTestExecution(t, db)

	_, err := db.Exec(`
		INSERT INTO execution_claims (execution_id, claimed_by, lease_expires_at)
		VALUES ($1, 'dead-node', NOW() - INTERVAL '1 second')
	`, execID)
	require.NoError(t, err)

	released, err := r.ReapExecutionClaims(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, released)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/engine/ -run TestReaper -v`
Expected: FAIL — `Reaper` type doesn't exist.

- [ ] **Step 3: Write the Reaper implementation**

Create `internal/engine/reaper.go`:

```go
package engine

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

// Reaper reclaims orphaned steps and execution claims with expired leases.
type Reaper struct {
	DB       *sql.DB
	Interval time.Duration
	Logger   *slog.Logger
}

// ReapSteps marks running steps with expired leases as failed.
// The orchestrator is responsible for creating retry attempts.
func (r *Reaper) ReapSteps(ctx context.Context) (int, error) {
	result, err := r.DB.ExecContext(ctx, `
		UPDATE step_executions
		SET status = 'failed', error = 'lease expired',
		    completed_at = NOW(), claimed_by = NULL, lease_expires_at = NULL,
		    updated_at = NOW()
		WHERE status = 'running'
		  AND lease_expires_at < NOW()
	`)
	if err != nil {
		return 0, fmt.Errorf("reap expired steps: %w", err)
	}

	rows, _ := result.RowsAffected()
	return int(rows), nil
}

// ReapExecutionClaims releases execution claims with expired leases.
func (r *Reaper) ReapExecutionClaims(ctx context.Context) (int, error) {
	result, err := r.DB.ExecContext(ctx, `
		DELETE FROM execution_claims
		WHERE lease_expires_at < NOW()
	`)
	if err != nil {
		return 0, fmt.Errorf("reap expired execution claims: %w", err)
	}

	rows, _ := result.RowsAffected()
	return int(rows), nil
}

// Run starts the reaper loop. Blocks until ctx is cancelled.
func (r *Reaper) Run(ctx context.Context) {
	stepTicker := time.NewTicker(r.Interval)
	defer stepTicker.Stop()

	// Offset execution claim reaping by half the interval (cancellable)
	offsetTimer := time.NewTimer(r.Interval / 2)
	select {
	case <-ctx.Done():
		offsetTimer.Stop()
		return
	case <-offsetTimer.C:
	}
	claimTicker := time.NewTicker(r.Interval)
	defer claimTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-stepTicker.C:
			reclaimed, err := r.ReapSteps(ctx)
			if err != nil {
				r.Logger.Error("reaper: step reap failed", "error", err)
			} else if reclaimed > 0 {
				r.Logger.Info("reaper: reclaimed expired steps", "count", reclaimed)
			}
		case <-claimTicker.C:
			released, err := r.ReapExecutionClaims(ctx)
			if err != nil {
				r.Logger.Error("reaper: execution claim reap failed", "error", err)
			} else if released > 0 {
				r.Logger.Info("reaper: released expired execution claims", "count", released)
			}
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/engine/ -run TestReaper -v`
Expected: All reaper tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/engine/reaper.go internal/engine/reaper_test.go
git commit -m "feat(phase7): add reaper for expired step leases and execution claims"
```

---

## Task 5: Orchestrator Loop

**Files:**
- Create: `internal/engine/orchestrator.go`
- Create: `internal/engine/orchestrator_test.go`
- Modify: `internal/workflow/workflow.go` — add `DependsOn` field

- [ ] **Step 1: Add DependsOn to Step struct**

In `internal/workflow/workflow.go`, add to Step:

```go
type Step struct {
	Name       string         `yaml:"name"`
	Action     string         `yaml:"action"`
	Params     map[string]any `yaml:"params"`
	If         string         `yaml:"if"`
	Retry      *RetryPolicy   `yaml:"retry"`
	Timeout    string         `yaml:"timeout"`
	Credential string         `yaml:"credential"`
	DependsOn  []string       `yaml:"depends_on"`
}
```

- [ ] **Step 2: Write the failing orchestrator tests**

Create `internal/engine/orchestrator_test.go`:

```go
package engine

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrchestrator_ClaimsExecution(t *testing.T) {
	db := setupTestDB(t)
	o := &Orchestrator{
		DB:            db,
		NodeID:        "node-1",
		LeaseDuration: 120 * time.Second,
	}

	execID := createTestExecution(t, db)

	claimed, err := o.ClaimExecution(context.Background(), execID)
	require.NoError(t, err)
	assert.True(t, claimed)

	// Second claim attempt fails
	claimed2, err := o.ClaimExecution(context.Background(), execID)
	require.NoError(t, err)
	assert.False(t, claimed2)
}

func TestOrchestrator_RenewExecutionLease(t *testing.T) {
	db := setupTestDB(t)
	o := &Orchestrator{
		DB:            db,
		NodeID:        "node-1",
		LeaseDuration: 120 * time.Second,
	}

	execID := createTestExecution(t, db)
	o.ClaimExecution(context.Background(), execID)

	ok, err := o.RenewExecutionLease(context.Background(), execID)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestOrchestrator_ReleaseExecution(t *testing.T) {
	db := setupTestDB(t)
	o := &Orchestrator{
		DB:            db,
		NodeID:        "node-1",
		LeaseDuration: 120 * time.Second,
	}

	execID := createTestExecution(t, db)
	o.ClaimExecution(context.Background(), execID)

	err := o.ReleaseExecution(context.Background(), execID)
	require.NoError(t, err)

	// Can claim again after release
	claimed, _ := o.ClaimExecution(context.Background(), execID)
	assert.True(t, claimed)
}

func TestOrchestrator_CreatePendingSteps(t *testing.T) {
	db := setupTestDB(t)
	o := &Orchestrator{DB: db, NodeID: "node-1", LeaseDuration: 120 * time.Second}

	execID := createTestExecution(t, db)

	stepNames := []string{"step-a", "step-b"}
	err := o.CreatePendingSteps(context.Background(), execID, stepNames, map[string]int{"step-a": 3, "step-b": 1})
	require.NoError(t, err)

	var count int
	db.QueryRow(`SELECT COUNT(*) FROM step_executions WHERE execution_id = $1 AND status = 'pending'`, execID).Scan(&count)
	assert.Equal(t, 2, count)
}

func TestOrchestrator_GetStepStatuses(t *testing.T) {
	db := setupTestDB(t)
	o := &Orchestrator{DB: db, NodeID: "node-1", LeaseDuration: 120 * time.Second}

	execID := createTestExecution(t, db)
	insertPendingStep(t, db, execID, "step-a", 1)

	statuses, err := o.GetStepStatuses(context.Background(), execID)
	require.NoError(t, err)
	assert.Equal(t, "pending", statuses["step-a"].Status)
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/engine/ -run TestOrchestrator -v`
Expected: FAIL — `Orchestrator` type doesn't exist.

- [ ] **Step 4: Write the Orchestrator implementation**

Create `internal/engine/orchestrator.go`:

```go
package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

// StepStatus represents the current status of a step execution.
type StepStatus struct {
	Status      string
	Attempt     int
	MaxAttempts int
	Output      map[string]any
	Error       string
}

// Orchestrator manages execution-level orchestration: claiming executions,
// creating pending steps based on DAG resolution, and monitoring completion.
type Orchestrator struct {
	DB            *sql.DB
	NodeID        string
	LeaseDuration time.Duration
	PollInterval  time.Duration
	Logger        *slog.Logger
}

// ClaimExecution attempts to claim orchestration of an execution.
// Returns true if this node won the claim, false if another node already has it.
func (o *Orchestrator) ClaimExecution(ctx context.Context, executionID string) (bool, error) {
	result, err := o.DB.ExecContext(ctx, `
		INSERT INTO execution_claims (execution_id, claimed_by, lease_expires_at)
		VALUES ($1, $2, NOW() + $3::interval)
		ON CONFLICT (execution_id) DO NOTHING
	`, executionID, o.NodeID, fmt.Sprintf("%d seconds", int(o.LeaseDuration.Seconds())))
	if err != nil {
		return false, fmt.Errorf("claim execution: %w", err)
	}

	rows, _ := result.RowsAffected()
	return rows > 0, nil
}

// RenewExecutionLease extends the lease on an orchestrated execution.
func (o *Orchestrator) RenewExecutionLease(ctx context.Context, executionID string) (bool, error) {
	result, err := o.DB.ExecContext(ctx, `
		UPDATE execution_claims
		SET lease_expires_at = NOW() + $2::interval
		WHERE execution_id = $1 AND claimed_by = $3
	`, executionID, fmt.Sprintf("%d seconds", int(o.LeaseDuration.Seconds())), o.NodeID)
	if err != nil {
		return false, fmt.Errorf("renew execution lease: %w", err)
	}

	rows, _ := result.RowsAffected()
	return rows > 0, nil
}

// ReleaseExecution removes the orchestration claim for an execution.
func (o *Orchestrator) ReleaseExecution(ctx context.Context, executionID string) error {
	_, err := o.DB.ExecContext(ctx, `
		DELETE FROM execution_claims WHERE execution_id = $1 AND claimed_by = $2
	`, executionID, o.NodeID)
	return err
}

// CreatePendingSteps inserts pending step_execution rows for the given step names.
// Uses ON CONFLICT DO NOTHING for idempotency on crash recovery.
func (o *Orchestrator) CreatePendingSteps(ctx context.Context, executionID string, stepNames []string, maxAttempts map[string]int) error {
	for _, name := range stepNames {
		ma := 1
		if v, ok := maxAttempts[name]; ok {
			ma = v
		}
		_, err := o.DB.ExecContext(ctx, `
			INSERT INTO step_executions (execution_id, step_name, attempt, status, max_attempts)
			VALUES ($1, $2, 1, 'pending', $3)
			ON CONFLICT (execution_id, step_name, attempt) DO NOTHING
		`, executionID, name, ma)
		if err != nil {
			return fmt.Errorf("create pending step %s: %w", name, err)
		}
	}
	return nil
}

// CreateRetryStep creates a new step_execution row for a retry attempt.
func (o *Orchestrator) CreateRetryStep(ctx context.Context, executionID, stepName string, nextAttempt, maxAttempts int) error {
	_, err := o.DB.ExecContext(ctx, `
		INSERT INTO step_executions (execution_id, step_name, attempt, status, max_attempts)
		VALUES ($1, $2, $3, 'pending', $4)
		ON CONFLICT (execution_id, step_name, attempt) DO NOTHING
	`, executionID, stepName, nextAttempt, maxAttempts)
	return err
}

// GetStepStatuses returns the latest status for each step name in an execution.
func (o *Orchestrator) GetStepStatuses(ctx context.Context, executionID string) (map[string]*StepStatus, error) {
	rows, err := o.DB.QueryContext(ctx, `
		SELECT DISTINCT ON (step_name) step_name, status, attempt, max_attempts, output, error
		FROM step_executions
		WHERE execution_id = $1 AND parent_step_id IS NULL
		ORDER BY step_name, attempt DESC
	`, executionID)
	if err != nil {
		return nil, fmt.Errorf("get step statuses: %w", err)
	}
	defer rows.Close()

	statuses := make(map[string]*StepStatus)
	for rows.Next() {
		var name, status string
		var attempt, maxAttempts int
		var outputJSON sql.NullString
		var errMsg sql.NullString

		if err := rows.Scan(&name, &status, &attempt, &maxAttempts, &outputJSON, &errMsg); err != nil {
			return nil, err
		}

		ss := &StepStatus{
			Status:      status,
			Attempt:     attempt,
			MaxAttempts: maxAttempts,
		}
		if outputJSON.Valid {
			json.Unmarshal([]byte(outputJSON.String), &ss.Output)
		}
		if errMsg.Valid {
			ss.Error = errMsg.String
		}
		statuses[name] = ss
	}
	return statuses, rows.Err()
}

// CancelPendingSteps marks all pending steps for an execution as cancelled.
func (o *Orchestrator) CancelPendingSteps(ctx context.Context, executionID string, stepNames []string) error {
	for _, name := range stepNames {
		_, err := o.DB.ExecContext(ctx, `
			UPDATE step_executions
			SET status = 'cancelled', completed_at = NOW(), updated_at = NOW()
			WHERE execution_id = $1 AND step_name = $2 AND status = 'pending'
		`, executionID, name)
		if err != nil {
			return fmt.Errorf("cancel step %s: %w", name, err)
		}
	}
	return nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/engine/ -run TestOrchestrator -v`
Expected: All orchestrator tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/engine/orchestrator.go internal/engine/orchestrator_test.go internal/workflow/workflow.go
git commit -m "feat(phase7): add execution orchestrator with claim/release/retry logic"
```

---

## Task 6: Worker Loop

**Files:**
- Create: `internal/engine/worker.go`
- Create: `internal/engine/worker_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/engine/worker_test.go`:

```go
package engine

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorker_ExecutesPendingStep(t *testing.T) {
	db := setupTestDB(t)

	execID := createTestExecution(t, db)
	insertPendingStep(t, db, execID, "step-a", 1)

	var executed atomic.Bool
	mockExecutor := func(ctx context.Context, stepName string, attempt int) (map[string]any, error) {
		executed.Store(true)
		return map[string]any{"result": "ok"}, nil
	}

	w := &Worker{
		Claimer:      &Claimer{DB: db, NodeID: "node-1", LeaseDuration: 60 * time.Second},
		StepExecutor: mockExecutor,
		PollInterval: 50 * time.Millisecond,
		MaxBackoff:   200 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go w.Run(ctx)

	// Wait for execution
	require.Eventually(t, func() bool { return executed.Load() }, 2*time.Second, 50*time.Millisecond)

	// Verify step is completed
	var status string
	db.QueryRow(`SELECT status FROM step_executions WHERE execution_id = $1 AND step_name = 'step-a'`, execID).Scan(&status)
	assert.Equal(t, "completed", status)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/engine/ -run TestWorker -v`
Expected: FAIL — `Worker` type doesn't exist.

- [ ] **Step 3: Write the Worker implementation**

Create `internal/engine/worker.go`:

```go
package engine

import (
	"context"
	"log/slog"
	"math/rand"
	"time"
)

// StepExecutor is a function that executes a step and returns its output.
type StepExecutor func(ctx context.Context, stepName string, attempt int) (map[string]any, error)

// Worker polls for pending steps, claims them, and executes them.
type Worker struct {
	Claimer       *Claimer
	StepExecutor  StepExecutor
	PollInterval  time.Duration
	MaxBackoff    time.Duration
	LeaseRenewInterval time.Duration
	Logger        *slog.Logger
}

// Run starts the worker loop. Blocks until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	backoff := w.PollInterval

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		claim, _, err := w.Claimer.ClaimAnyStep(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if w.Logger != nil {
				w.Logger.Error("worker: claim failed", "error", err)
			}
			w.sleep(ctx, backoff)
			continue
		}

		if claim == nil {
			// No work available — back off
			backoff = w.nextBackoff(backoff)
			w.sleep(ctx, backoff)
			continue
		}

		// Reset backoff on successful claim
		backoff = w.PollInterval

		// Execute the step with lease renewal
		w.executeWithLeaseRenewal(ctx, claim)
	}
}

func (w *Worker) executeWithLeaseRenewal(ctx context.Context, claim *StepClaim) {
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Start lease renewal goroutine
	renewInterval := w.LeaseRenewInterval
	if renewInterval == 0 {
		renewInterval = w.Claimer.LeaseDuration / 3
	}

	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(renewInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-execCtx.Done():
				return
			case <-ticker.C:
				ok, err := w.Claimer.RenewLease(ctx, claim.ID)
				if err != nil || !ok {
					// Lease lost — cancel execution
					cancel()
					return
				}
			}
		}
	}()

	// Execute the step
	output, stepErr := w.StepExecutor(execCtx, claim.StepName, claim.Attempt)
	close(done)

	// Complete the step (fencing token protects against stale completion)
	ok, err := w.Claimer.CompleteStep(ctx, claim.ID, output, stepErr)
	if err != nil && w.Logger != nil {
		w.Logger.Error("worker: complete step failed", "step", claim.StepName, "error", err)
	}
	if !ok && w.Logger != nil {
		w.Logger.Warn("worker: step completion rejected (fencing)", "step", claim.StepName)
	}
}

func (w *Worker) nextBackoff(current time.Duration) time.Duration {
	next := current * 2
	if next > w.MaxBackoff {
		next = w.MaxBackoff
	}
	// Add jitter ±25%
	jitter := time.Duration(float64(next) * (0.75 + rand.Float64()*0.5))
	return jitter
}

func (w *Worker) sleep(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/engine/ -run TestWorker -v`
Expected: All worker tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/engine/worker.go internal/engine/worker_test.go
git commit -m "feat(phase7): add worker loop with poll/backoff and lease renewal"
```

---

## Task 7: Metrics

**Files:**
- Modify: `internal/metrics/metrics.go`
- Modify: `internal/metrics/metrics_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/metrics/metrics_test.go`:

```go
func TestQueueMetrics(t *testing.T) {
	SetQueueDepth(5)
	RecordClaimDuration(100 * time.Millisecond)
	RecordLeaseRenewal()
	RecordLeaseExpiration()
	RecordReaperReclaimed(3)
	// Just verify they don't panic — Prometheus registration is the real test
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/metrics/ -run TestQueueMetrics -v`
Expected: FAIL — functions don't exist.

- [ ] **Step 3: Add queue metrics**

In `internal/metrics/metrics.go`, add:

```go
var (
	QueueDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "mantle_queue_depth",
		Help: "Number of pending steps in the work queue",
	})
	ClaimDurationSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "mantle_claim_duration_seconds",
		Help:    "Time from step pending to claimed",
		Buckets: prometheus.DefBuckets,
	})
	LeaseRenewalsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "mantle_lease_renewals_total",
		Help: "Total number of lease renewals",
	})
	LeaseExpirationsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "mantle_lease_expirations_total",
		Help: "Total number of lease expirations (indicates node failures)",
	})
	ReaperReclaimedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "mantle_reaper_reclaimed_total",
		Help: "Total number of steps reclaimed by reaper",
	})
)

func SetQueueDepth(n int) {
	QueueDepth.Set(float64(n))
}

func RecordClaimDuration(d time.Duration) {
	ClaimDurationSeconds.Observe(d.Seconds())
}

func RecordLeaseRenewal() {
	LeaseRenewalsTotal.Inc()
}

func RecordLeaseExpiration() {
	LeaseExpirationsTotal.Inc()
}

func RecordReaperReclaimed(n int) {
	ReaperReclaimedTotal.Add(float64(n))
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/metrics/ -v`
Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/metrics/metrics.go internal/metrics/metrics_test.go
git commit -m "feat(phase7): add Prometheus metrics for queue depth, claims, leases, reaper"
```

---

## Task 8: Engine Refactor for Distributed Mode

**Files:**
- Modify: `internal/engine/engine.go`

- [ ] **Step 1: Add output size enforcement**

In `internal/engine/engine.go`, add an output size check after step execution and before `updateStep`. Add a helper:

```go
const maxStepOutputBytes = 1048576 // 1MB default

func (e *Engine) checkOutputSize(output map[string]any, maxBytes int) error {
	data, err := json.Marshal(output)
	if err != nil {
		return fmt.Errorf("marshal output for size check: %w", err)
	}
	if len(data) > maxBytes {
		return fmt.Errorf("step output exceeds %d byte limit (%d bytes). Use the S3 connector to store large payloads and pass the key to downstream steps", maxBytes, len(data))
	}
	return nil
}
```

Call this after `conn.Execute()` returns in `executeStep`, before writing to the database.

- [ ] **Step 2: Add StepExecutorFunc that bridges old engine to new worker**

Add a method that creates a `StepExecutor` function from the engine's existing connector/CEL/secret machinery:

```go
func (e *Engine) MakeStepExecutor(wf *workflow.Workflow, execID string, celCtx *mantleCEL.Context) StepExecutor {
	return func(ctx context.Context, stepName string, attempt int) (map[string]any, error) {
		step := wf.FindStep(stepName)
		if step == nil {
			return nil, fmt.Errorf("step %q not found in workflow", stepName)
		}
		return e.executeStepLogic(ctx, execID, *step, celCtx)
	}
}
```

Extract the core connector invocation logic from `executeStep` into `executeStepLogic` so it can be called by both the old sequential path and the new worker path.

- [ ] **Step 3: Add FindStep helper to workflow**

In `internal/workflow/workflow.go`:

```go
func (w *Workflow) FindStep(name string) *Step {
	for i := range w.Steps {
		if w.Steps[i].Name == name {
			return &w.Steps[i]
		}
	}
	return nil
}
```

- [ ] **Step 4: Run existing tests to ensure nothing is broken**

Run: `go test ./internal/engine/ -v`
Expected: All existing tests still pass.

- [ ] **Step 5: Commit**

```bash
git add internal/engine/engine.go internal/workflow/workflow.go
git commit -m "feat(phase7): refactor engine for distributed step execution"
```

---

## Task 9: Server Integration

**Files:**
- Modify: `internal/server/server.go`

- [ ] **Step 1: Start orchestrator, worker, and reaper in serve mode**

In the `Server.Start` method, after the existing setup, start the distributed components:

```go
claimer := &engine.Claimer{
	DB:            s.DB,
	NodeID:        cfg.Engine.NodeID,
	LeaseDuration: cfg.Engine.StepLeaseDuration,
}

// Create a StepExecutor that bridges the engine's connector/CEL/secret machinery
// to the worker loop. The executor loads the workflow definition and step config,
// then delegates to engine.executeStepLogic.
stepExecutor := s.Engine.MakeGlobalStepExecutor()

// Start worker loop
worker := &engine.Worker{
	Claimer:            claimer,
	StepExecutor:       stepExecutor,
	PollInterval:       cfg.Engine.WorkerPollInterval,
	MaxBackoff:         cfg.Engine.WorkerMaxBackoff,
	LeaseRenewInterval: cfg.Engine.StepLeaseDuration / 3,
	Logger:             s.Logger,
}
go worker.Run(ctx)

// Start reaper
reaper := &engine.Reaper{
	DB:       s.DB,
	Interval: cfg.Engine.ReaperInterval,
	Logger:   s.Logger,
}
go reaper.Run(ctx)
```

The `MakeGlobalStepExecutor` method returns a `StepExecutor` that:
1. Looks up the step's execution_id to find the workflow definition
2. Loads the workflow and builds a CEL context from completed step outputs
3. Delegates to `executeStepLogic` for the actual connector invocation

For Phase 7, the existing `Engine.Execute` path is modified to create step_execution rows as `'pending'` instead of executing them inline. The orchestrator loop in Phase 7 creates steps sequentially (no DAG — all steps depend on the previous one). Phase 8 upgrades this to DAG-based scheduling.

**Important**: The `Engine.Execute` method (used by `mantle run` and `handleRun`) must be updated to:
1. Create a workflow_execution row
2. Claim the execution via the Orchestrator
3. Create pending step_execution rows for the first batch of ready steps
4. Return — the worker loop handles actual execution

This replaces the current inline sequential execution when running in distributed mode (server mode). The CLI `mantle run` path can keep the old sequential behavior for simplicity.

- [ ] **Step 2: Run existing server tests**

Run: `go test ./internal/server/ -v`
Expected: All existing tests pass.

- [ ] **Step 3: Commit**

```bash
git add internal/server/server.go
git commit -m "feat(phase7): start worker and reaper goroutines in server mode"
```

---

## Task 10: Concurrency Integration Tests

**Files:**
- Create: `internal/engine/distributed_test.go`

- [ ] **Step 1: Write the multi-worker concurrency test**

```go
package engine

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDistributed_NoStepDuplication(t *testing.T) {
	db := setupTestDB(t)

	execID := createTestExecution(t, db)
	for i := 0; i < 10; i++ {
		insertPendingStep(t, db, execID, fmt.Sprintf("step-%d", i), 1)
	}

	var executionCount atomic.Int32
	executor := func(ctx context.Context, stepName string, attempt int) (map[string]any, error) {
		executionCount.Add(1)
		time.Sleep(10 * time.Millisecond) // Simulate work
		return map[string]any{"step": stepName}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start 3 workers
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(nodeID string) {
			defer wg.Done()
			w := &Worker{
				Claimer:      &Claimer{DB: db, NodeID: nodeID, LeaseDuration: 60 * time.Second},
				StepExecutor: executor,
				PollInterval: 50 * time.Millisecond,
				MaxBackoff:   200 * time.Millisecond,
			}
			w.Run(ctx)
		}(fmt.Sprintf("node-%d", i))
	}

	// Wait until all steps are completed
	require.Eventually(t, func() bool {
		var count int
		db.QueryRow(`SELECT COUNT(*) FROM step_executions WHERE execution_id = $1 AND status = 'completed'`, execID).Scan(&count)
		return count == 10
	}, 10*time.Second, 100*time.Millisecond)

	cancel()
	wg.Wait()

	// Verify exactly 10 executions (no duplication)
	assert.Equal(t, int32(10), executionCount.Load())
}

func TestDistributed_CrashRecovery(t *testing.T) {
	db := setupTestDB(t)

	execID := createTestExecution(t, db)
	insertPendingStep(t, db, execID, "crash-step", 1)

	// Simulate a crashed worker: claim with expired lease
	_, err := db.Exec(`
		UPDATE step_executions
		SET status = 'running', claimed_by = 'dead-node',
		    lease_expires_at = NOW() - INTERVAL '1 second',
		    started_at = NOW(), max_attempts = 3
		WHERE execution_id = $1 AND step_name = 'crash-step'
	`, execID)
	require.NoError(t, err)

	// Run reaper to reclaim
	reaper := &Reaper{DB: db}
	reclaimed, err := reaper.ReapSteps(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, reclaimed)

	// Verify step is now failed
	var status string
	db.QueryRow(`SELECT status FROM step_executions WHERE execution_id = $1 AND step_name = 'crash-step' AND attempt = 1`, execID).Scan(&status)
	assert.Equal(t, "failed", status)

	// Orchestrator would create retry attempt
	o := &Orchestrator{DB: db, NodeID: "node-1", LeaseDuration: 120 * time.Second}
	err = o.CreateRetryStep(context.Background(), execID, "crash-step", 2, 3)
	require.NoError(t, err)

	// New worker picks up retry
	var executed atomic.Bool
	executor := func(ctx context.Context, stepName string, attempt int) (map[string]any, error) {
		executed.Store(true)
		assert.Equal(t, 2, attempt)
		return map[string]any{"recovered": true}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	w := &Worker{
		Claimer:      &Claimer{DB: db, NodeID: "node-2", LeaseDuration: 60 * time.Second},
		StepExecutor: executor,
		PollInterval: 50 * time.Millisecond,
		MaxBackoff:   200 * time.Millisecond,
	}
	go w.Run(ctx)

	require.Eventually(t, func() bool { return executed.Load() }, 5*time.Second, 50*time.Millisecond)
	cancel()
}
```

- [ ] **Step 2: Run the concurrency tests**

Run: `go test ./internal/engine/ -run TestDistributed -v -timeout 30s`
Expected: All tests pass — no step duplication, crash recovery works.

- [ ] **Step 3: Commit**

```bash
git add internal/engine/distributed_test.go
git commit -m "test(phase7): add concurrency and crash recovery integration tests"
```

---

## Task 11: Run Full Test Suite

- [ ] **Step 1: Run all tests**

Run: `make test`
Expected: All tests pass.

- [ ] **Step 2: Run linter**

Run: `make lint`
Expected: No new lint errors.

- [ ] **Step 3: Final commit if any cleanup needed**

```bash
git add -A
git commit -m "chore(phase7): final cleanup after full test suite"
```
