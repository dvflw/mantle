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
