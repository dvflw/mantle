-- +goose Up
CREATE TABLE workflow_triggers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_name TEXT NOT NULL,
    workflow_version INTEGER NOT NULL,
    type TEXT NOT NULL,
    schedule TEXT,
    path TEXT,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_workflow_triggers_type_enabled ON workflow_triggers (type, enabled);
CREATE UNIQUE INDEX idx_workflow_triggers_webhook_path ON workflow_triggers (path) WHERE type = 'webhook' AND enabled = true;
CREATE UNIQUE INDEX idx_workflow_triggers_cron ON workflow_triggers (workflow_name, schedule) WHERE type = 'cron';

-- +goose Down
DROP INDEX IF EXISTS idx_workflow_triggers_cron;
DROP INDEX IF EXISTS idx_workflow_triggers_webhook_path;
DROP INDEX IF EXISTS idx_workflow_triggers_type_enabled;
DROP TABLE IF EXISTS "workflow_triggers";
