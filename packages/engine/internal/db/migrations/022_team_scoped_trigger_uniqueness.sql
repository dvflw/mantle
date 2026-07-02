-- +goose Up
-- 022_team_scoped_trigger_uniqueness: trigger uniqueness must be per-team.
--
-- Workflow names are team-scoped (workflow_definitions is UNIQUE(team_id, name,
-- version)) and the /hooks/ endpoint is authenticated, so trigger lookups always
-- run under the caller's team. The original uniqueness indexes were global, so
-- once trigger registration went live a second team registering the same cron
-- (workflow_name, schedule) or the same webhook path would fail its apply with a
-- unique-constraint violation. Scope both by team_id; the caller's team
-- disambiguates at lookup time. Email triggers have no uniqueness index.
DROP INDEX IF EXISTS idx_workflow_triggers_cron;
CREATE UNIQUE INDEX idx_workflow_triggers_cron
    ON workflow_triggers (team_id, workflow_name, schedule)
    WHERE type = 'cron';

DROP INDEX IF EXISTS idx_workflow_triggers_webhook_path;
CREATE UNIQUE INDEX idx_workflow_triggers_webhook_path
    ON workflow_triggers (team_id, path)
    WHERE type = 'webhook' AND enabled = true;

-- +goose Down
-- NOTE: this rollback recreates the original GLOBAL unique indexes. If two teams
-- have registered the same cron (workflow_name, schedule) or webhook path while
-- the team-scoped indexes were in effect, recreating the global indexes will
-- fail with a duplicate-key violation. That is an accepted rollback limitation:
-- an operator must resolve the cross-team duplicates before downgrading.
DROP INDEX IF EXISTS idx_workflow_triggers_webhook_path;
CREATE UNIQUE INDEX idx_workflow_triggers_webhook_path
    ON workflow_triggers (path)
    WHERE type = 'webhook' AND enabled = true;

DROP INDEX IF EXISTS idx_workflow_triggers_cron;
CREATE UNIQUE INDEX idx_workflow_triggers_cron
    ON workflow_triggers (workflow_name, schedule)
    WHERE type = 'cron';
