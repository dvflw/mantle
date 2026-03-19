-- +goose Up
ALTER TABLE step_executions ADD COLUMN claimed_by TEXT;
ALTER TABLE step_executions ADD COLUMN lease_expires_at TIMESTAMPTZ;
ALTER TABLE step_executions ADD COLUMN max_attempts INTEGER NOT NULL DEFAULT 3;
ALTER TABLE step_executions ADD COLUMN parent_step_id UUID REFERENCES step_executions(id);
ALTER TABLE step_executions ADD COLUMN cached_llm_responses JSONB;

CREATE INDEX idx_step_executions_claimable
    ON step_executions (execution_id, created_at)
    WHERE status = 'pending' AND claimed_by IS NULL;

CREATE TABLE execution_claims (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    step_execution_id UUID NOT NULL REFERENCES step_executions(id),
    node_id TEXT NOT NULL,
    claimed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    released_at TIMESTAMPTZ,
    outcome TEXT
);

CREATE INDEX idx_execution_claims_step ON execution_claims(step_execution_id);

-- +goose Down
DROP INDEX IF EXISTS idx_execution_claims_step;
DROP TABLE IF EXISTS execution_claims;
DROP INDEX IF EXISTS idx_step_executions_claimable;
ALTER TABLE step_executions DROP COLUMN IF EXISTS cached_llm_responses;
ALTER TABLE step_executions DROP COLUMN IF EXISTS parent_step_id;
ALTER TABLE step_executions DROP COLUMN IF EXISTS max_attempts;
ALTER TABLE step_executions DROP COLUMN IF EXISTS lease_expires_at;
ALTER TABLE step_executions DROP COLUMN IF EXISTS claimed_by;
