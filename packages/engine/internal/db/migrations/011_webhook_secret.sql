-- +goose Up
ALTER TABLE workflow_triggers ADD COLUMN secret TEXT;

-- +goose Down
ALTER TABLE workflow_triggers DROP COLUMN IF EXISTS secret;
