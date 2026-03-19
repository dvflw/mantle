-- +goose Up
ALTER TABLE step_executions ADD COLUMN parent_step_id UUID REFERENCES step_executions(id);
CREATE INDEX idx_step_executions_parent ON step_executions(parent_step_id) WHERE parent_step_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_step_executions_parent;
ALTER TABLE step_executions DROP COLUMN IF EXISTS parent_step_id;
