-- +goose Up
-- 022_team_scoped_trigger_uniqueness: cron trigger uniqueness must be per-team.
--
-- Workflow names are team-scoped (workflow_definitions is UNIQUE(team_id, name,
-- version)), so two teams may legitimately own a workflow of the same name on
-- the same schedule. The original index on (workflow_name, schedule) was global,
-- so the second team's apply failed with a unique-constraint violation once
-- trigger registration went live. Scope the uniqueness by team.
--
-- The webhook path index is intentionally left global: an inbound
-- POST /hooks/<path> request carries no team identity, so the path is the global
-- routing key and must be unique across all teams. Email triggers have no
-- uniqueness index and are unaffected.
DROP INDEX IF EXISTS idx_workflow_triggers_cron;
CREATE UNIQUE INDEX idx_workflow_triggers_cron
    ON workflow_triggers (team_id, workflow_name, schedule)
    WHERE type = 'cron';

-- +goose Down
DROP INDEX IF EXISTS idx_workflow_triggers_cron;
CREATE UNIQUE INDEX idx_workflow_triggers_cron
    ON workflow_triggers (workflow_name, schedule)
    WHERE type = 'cron';
