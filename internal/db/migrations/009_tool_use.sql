-- +goose Up
-- parent_step_id and cached_llm_responses already added in 007_multi_node.sql
-- Only add the parent index which was missing from that migration
CREATE INDEX IF NOT EXISTS idx_step_executions_parent ON step_executions(parent_step_id) WHERE parent_step_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_step_executions_parent;
