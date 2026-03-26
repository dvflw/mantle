-- +goose Up
ALTER TABLE workflow_triggers ADD COLUMN mailbox TEXT;
ALTER TABLE workflow_triggers ADD COLUMN folder TEXT DEFAULT 'INBOX';
ALTER TABLE workflow_triggers ADD COLUMN filter TEXT DEFAULT 'unseen';
ALTER TABLE workflow_triggers ADD COLUMN poll_interval TEXT DEFAULT '60s';

CREATE INDEX idx_workflow_triggers_email ON workflow_triggers (type, enabled)
    WHERE type = 'email';

-- +goose Down
DROP INDEX IF EXISTS idx_workflow_triggers_email;
ALTER TABLE workflow_triggers DROP COLUMN IF EXISTS poll_interval;
ALTER TABLE workflow_triggers DROP COLUMN IF EXISTS filter;
ALTER TABLE workflow_triggers DROP COLUMN IF EXISTS folder;
ALTER TABLE workflow_triggers DROP COLUMN IF EXISTS mailbox;
