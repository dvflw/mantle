-- +goose Up
ALTER TABLE step_executions ADD COLUMN continue_on_error BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE step_executions DROP COLUMN IF EXISTS continue_on_error;
