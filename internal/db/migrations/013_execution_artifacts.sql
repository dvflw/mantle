-- +goose Up
CREATE TABLE execution_artifacts (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id UUID NOT NULL REFERENCES workflow_executions(id) ON DELETE CASCADE,
    step_name    TEXT NOT NULL,
    name         TEXT NOT NULL,
    url          TEXT NOT NULL,
    size         BIGINT NOT NULL CHECK (size >= 0),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (execution_id, name)
);

CREATE INDEX idx_execution_artifacts_created_at ON execution_artifacts(created_at);

-- +goose Down
DROP TABLE IF EXISTS execution_artifacts;
