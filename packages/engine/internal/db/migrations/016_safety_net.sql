-- +goose Up

-- Concurrency controls (#49): per-team execution limit override.
ALTER TABLE teams ADD COLUMN max_concurrent_executions INT;

-- Retry from failed step (#48): link to original execution for traceability.
ALTER TABLE workflow_executions ADD COLUMN retried_from_execution_id UUID
  REFERENCES workflow_executions(id);

-- Lifecycle hooks (#30): distinguish hook steps from main steps.
ALTER TABLE step_executions ADD COLUMN hook_block TEXT;

-- Workflow rollback (#50): track which version was restored.
ALTER TABLE workflow_definitions ADD COLUMN rollback_of INT;

-- +goose Down
ALTER TABLE workflow_definitions DROP COLUMN IF EXISTS rollback_of;
ALTER TABLE step_executions DROP COLUMN IF EXISTS hook_block;
ALTER TABLE workflow_executions DROP COLUMN IF EXISTS retried_from_execution_id;
ALTER TABLE teams DROP COLUMN IF EXISTS max_concurrent_executions;
