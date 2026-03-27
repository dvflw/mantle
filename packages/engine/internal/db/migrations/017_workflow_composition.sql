-- +goose Up
ALTER TABLE workflow_executions ADD COLUMN parent_execution_id UUID REFERENCES workflow_executions(id);
ALTER TABLE workflow_executions ADD COLUMN parent_step_name TEXT;
ALTER TABLE workflow_executions ADD COLUMN depth INTEGER NOT NULL DEFAULT 0;
CREATE INDEX idx_workflow_executions_parent ON workflow_executions(parent_execution_id) WHERE parent_execution_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_workflow_executions_parent;
ALTER TABLE workflow_executions DROP COLUMN IF EXISTS depth;
ALTER TABLE workflow_executions DROP COLUMN IF EXISTS parent_step_name;
ALTER TABLE workflow_executions DROP COLUMN IF EXISTS parent_execution_id;
