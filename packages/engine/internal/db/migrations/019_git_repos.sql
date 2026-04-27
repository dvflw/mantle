-- +goose Up
-- git_repos stores configuration for GitOps workflow sources (issue #16).
-- Each row represents a remote git repository that Mantle will pull from
-- and whose .yaml/.yml files will be applied as workflow definitions.
-- Raw auth material lives in the credentials table; this row references
-- it by name only.
--
-- The `name` column is a human-readable identifier for CLI ergonomics
-- (`mantle repos status <name>`), unique per team. It does not derive
-- from the repo URL because multiple teams may share the same upstream.
--
-- credential is stored as a plain name reference (TEXT NOT NULL) rather
-- than a foreign key. A FK to credentials(team_id, name) would prevent
-- operators from registering a repo before creating the credential, which
-- is a common IaC pattern (define everything in one pass, then apply).
-- The sync engine resolves and validates the credential at sync time.
CREATE TABLE git_repos (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE RESTRICT DEFAULT '00000000-0000-0000-0000-000000000001',
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    branch TEXT NOT NULL DEFAULT 'main',
    path TEXT NOT NULL DEFAULT '/',
    poll_interval TEXT NOT NULL DEFAULT '60s',
    credential TEXT NOT NULL,
    auto_apply BOOLEAN NOT NULL DEFAULT TRUE,
    prune BOOLEAN NOT NULL DEFAULT TRUE,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    last_sync_sha TEXT,
    last_sync_at TIMESTAMPTZ,
    last_sync_error TEXT,
    -- webhook_secret is intentionally absent in Plan A: the webhook receiver
    -- (Plan C) will add a new migration that stores the HMAC secret as an
    -- encrypted credential reference rather than plaintext TEXT.
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT git_repos_team_name_key UNIQUE(team_id, name)
);

CREATE INDEX idx_git_repos_team ON git_repos(team_id);
CREATE INDEX idx_git_repos_team_enabled ON git_repos(team_id, enabled) WHERE enabled = TRUE;

-- +goose Down
DROP TABLE IF EXISTS git_repos;
