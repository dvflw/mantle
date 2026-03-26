-- +goose Up

-- Index for ClaimAnyStep global query (worker poll across all executions)
CREATE INDEX idx_step_executions_claimable_any
  ON step_executions (created_at ASC)
  WHERE status = 'pending' AND claimed_by IS NULL AND parent_step_id IS NULL;

-- Index for sub-step queries by parent
CREATE INDEX idx_step_executions_parent
  ON step_executions (parent_step_id)
  WHERE parent_step_id IS NOT NULL;

-- Index for workflow execution listing API
CREATE INDEX idx_workflow_executions_started
  ON workflow_executions (started_at DESC NULLS LAST);

-- CHECK constraints for status columns
ALTER TABLE workflow_executions
  ADD CONSTRAINT chk_execution_status
  CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled'));

ALTER TABLE step_executions
  ADD CONSTRAINT chk_step_status
  CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled', 'skipped'));

-- +goose Down
ALTER TABLE step_executions DROP CONSTRAINT IF EXISTS chk_step_status;
ALTER TABLE workflow_executions DROP CONSTRAINT IF EXISTS chk_execution_status;
DROP INDEX IF EXISTS idx_workflow_executions_started;
DROP INDEX IF EXISTS idx_step_executions_parent;
DROP INDEX IF EXISTS idx_step_executions_claimable_any;
