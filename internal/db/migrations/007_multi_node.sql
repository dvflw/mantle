-- +goose Up
ALTER TABLE step_executions
    ADD COLUMN claimed_by TEXT,
    ADD COLUMN lease_expires_at TIMESTAMPTZ,
    ADD COLUMN max_attempts INTEGER NOT NULL DEFAULT 3;

CREATE INDEX idx_step_executions_lease ON step_executions (lease_expires_at)
    WHERE status = 'running' AND lease_expires_at IS NOT NULL;

CREATE TABLE execution_claims (
    execution_id UUID PRIMARY KEY REFERENCES workflow_executions(id),
    claimed_by TEXT NOT NULL,
    lease_expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_execution_claims_lease ON execution_claims (lease_expires_at);

-- +goose Down
DROP TABLE IF EXISTS execution_claims;
DROP INDEX IF EXISTS idx_step_executions_lease;
ALTER TABLE step_executions
    DROP COLUMN IF EXISTS claimed_by,
    DROP COLUMN IF EXISTS lease_expires_at,
    DROP COLUMN IF EXISTS max_attempts;
