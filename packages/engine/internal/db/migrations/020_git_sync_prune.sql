-- +goose Up
-- 020_git_sync_prune: adds the columns Plan C needs to implement prune
-- behavior for GitOps sync (issue #16).
--
-- workflow_definitions.disabled_at marks a version as inactive without
-- losing history. The sync engine sets it when a repo is pruning and
-- the workflow's source file has been removed upstream; clears it when
-- the file reappears.
--
-- git_repo_workflows tracks which workflows each repo "owns" so prune
-- does not accidentally disable workflows that were applied through
-- the CLI or from a different repo.
ALTER TABLE workflow_definitions
    ADD COLUMN disabled_at TIMESTAMPTZ;

CREATE INDEX idx_workflow_definitions_disabled
    ON workflow_definitions(disabled_at)
    WHERE disabled_at IS NOT NULL;

CREATE TABLE git_repo_workflows (
    repo_id UUID NOT NULL REFERENCES git_repos(id) ON DELETE CASCADE,
    workflow_name TEXT NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (repo_id, workflow_name)
);

CREATE INDEX idx_git_repo_workflows_repo ON git_repo_workflows(repo_id);

-- +goose Down
DROP TABLE IF EXISTS git_repo_workflows;
DROP INDEX IF EXISTS idx_workflow_definitions_disabled;
ALTER TABLE workflow_definitions DROP COLUMN IF EXISTS disabled_at;
