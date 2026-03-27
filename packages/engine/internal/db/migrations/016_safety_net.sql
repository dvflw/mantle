-- +goose Up

-- Concurrency controls (#49): add 'queued' and 'timed_out' to execution status check.
ALTER TABLE workflow_executions DROP CONSTRAINT IF EXISTS chk_execution_status;
ALTER TABLE workflow_executions
  ADD CONSTRAINT chk_execution_status
  CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled', 'queued', 'timed_out'));

-- Concurrency controls (#49): per-team execution limit override.
ALTER TABLE teams ADD COLUMN max_concurrent_executions INT;

-- Retry from failed step (#48): link to original execution for traceability.
ALTER TABLE workflow_executions ADD COLUMN retried_from_execution_id UUID
  REFERENCES workflow_executions(id);

-- Step-level timeout detection: add 'timed_out' to step status check.
ALTER TABLE step_executions DROP CONSTRAINT IF EXISTS chk_step_status;
ALTER TABLE step_executions
  ADD CONSTRAINT chk_step_status
  CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled', 'skipped', 'timed_out'));

-- Lifecycle hooks (#30): distinguish hook steps from main steps.
ALTER TABLE step_executions ADD COLUMN hook_block TEXT;

-- Replace the monolithic unique constraint with two partial indexes so that
-- hook steps can reuse the same step_name as main steps (disambiguated by hook_block).
ALTER TABLE step_executions DROP CONSTRAINT IF EXISTS step_executions_execution_id_step_name_attempt_key;
CREATE UNIQUE INDEX idx_step_executions_main_unique
  ON step_executions (execution_id, step_name, attempt) WHERE hook_block IS NULL;
CREATE UNIQUE INDEX idx_step_executions_hook_unique
  ON step_executions (execution_id, step_name, attempt, hook_block) WHERE hook_block IS NOT NULL;

-- Workflow rollback (#50): track which version was restored.
ALTER TABLE workflow_definitions ADD COLUMN rollback_of INT;

-- Index for retry lookups.
CREATE INDEX idx_workflow_executions_retried_from ON workflow_executions(retried_from_execution_id) WHERE retried_from_execution_id IS NOT NULL;

-- Partial index for hook vs main step filtering.
CREATE INDEX idx_step_executions_hook_block ON step_executions(execution_id, hook_block) WHERE hook_block IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_step_executions_hook_block;
DROP INDEX IF EXISTS idx_step_executions_hook_unique;
DROP INDEX IF EXISTS idx_step_executions_main_unique;
DROP INDEX IF EXISTS idx_workflow_executions_retried_from;
ALTER TABLE workflow_definitions DROP COLUMN IF EXISTS rollback_of;
-- Delete hook step rows before dropping the column so the restored unique
-- constraint does not conflict with rows that were hook-scoped.
DELETE FROM step_executions WHERE hook_block IS NOT NULL;
ALTER TABLE step_executions DROP COLUMN IF EXISTS hook_block;
-- Restore the original unique constraint (no partial index needed since hook rows are gone).
ALTER TABLE step_executions
  ADD CONSTRAINT step_executions_execution_id_step_name_attempt_key
  UNIQUE (execution_id, step_name, attempt);
ALTER TABLE workflow_executions DROP COLUMN IF EXISTS retried_from_execution_id;
ALTER TABLE teams DROP COLUMN IF EXISTS max_concurrent_executions;
-- Convert new statuses back to old equivalents so the CHECK constraints pass.
UPDATE workflow_executions SET status = 'pending' WHERE status = 'queued';
UPDATE workflow_executions SET status = 'failed' WHERE status = 'timed_out';
UPDATE step_executions SET status = 'failed' WHERE status = 'timed_out';
-- Restore original status constraints without queued/timed_out.
ALTER TABLE workflow_executions DROP CONSTRAINT IF EXISTS chk_execution_status;
ALTER TABLE workflow_executions
  ADD CONSTRAINT chk_execution_status
  CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled'));
ALTER TABLE step_executions DROP CONSTRAINT IF EXISTS chk_step_status;
ALTER TABLE step_executions
  ADD CONSTRAINT chk_step_status
  CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled', 'skipped'));
