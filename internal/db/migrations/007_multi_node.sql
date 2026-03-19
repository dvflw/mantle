-- +goose Up

-- Execution claims table for distributed node coordination
CREATE TABLE execution_claims (
    execution_id UUID PRIMARY KEY REFERENCES workflow_executions(id),
    claimed_by TEXT NOT NULL,
    lease_expires_at TIMESTAMPTZ NOT NULL,
    claimed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_execution_claims_expires ON execution_claims(lease_expires_at);

-- Add multi-node columns to step_executions
ALTER TABLE step_executions ADD COLUMN claimed_by TEXT;
ALTER TABLE step_executions ADD COLUMN lease_expires_at TIMESTAMPTZ;
ALTER TABLE step_executions ADD COLUMN max_attempts INTEGER NOT NULL DEFAULT 1;
ALTER TABLE step_executions ADD COLUMN parent_step_id UUID REFERENCES step_executions(id);

-- +goose Down

ALTER TABLE step_executions DROP COLUMN IF EXISTS parent_step_id;
ALTER TABLE step_executions DROP COLUMN IF EXISTS max_attempts;
ALTER TABLE step_executions DROP COLUMN IF EXISTS lease_expires_at;
ALTER TABLE step_executions DROP COLUMN IF EXISTS claimed_by;

DROP TABLE IF EXISTS execution_claims;
