-- +goose Up

-- Teams
CREATE TABLE teams (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Default team for single-tenant migration
INSERT INTO teams (id, name) VALUES ('00000000-0000-0000-0000-000000000001', 'default');

-- Users
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    team_id UUID NOT NULL REFERENCES teams(id),
    role TEXT NOT NULL DEFAULT 'operator',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- API keys (hashed, scoped to users)
CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    key_hash TEXT NOT NULL UNIQUE,
    key_prefix TEXT NOT NULL,
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Retrofit team_id on existing tables
ALTER TABLE workflow_definitions ADD COLUMN team_id UUID REFERENCES teams(id);
UPDATE workflow_definitions SET team_id = '00000000-0000-0000-0000-000000000001' WHERE team_id IS NULL;
ALTER TABLE workflow_definitions ALTER COLUMN team_id SET NOT NULL;
ALTER TABLE workflow_definitions ALTER COLUMN team_id SET DEFAULT '00000000-0000-0000-0000-000000000001';

ALTER TABLE workflow_executions ADD COLUMN team_id UUID REFERENCES teams(id);
UPDATE workflow_executions SET team_id = '00000000-0000-0000-0000-000000000001' WHERE team_id IS NULL;
ALTER TABLE workflow_executions ALTER COLUMN team_id SET NOT NULL;
ALTER TABLE workflow_executions ALTER COLUMN team_id SET DEFAULT '00000000-0000-0000-0000-000000000001';

ALTER TABLE credentials ADD COLUMN team_id UUID REFERENCES teams(id);
UPDATE credentials SET team_id = '00000000-0000-0000-0000-000000000001' WHERE team_id IS NULL;
ALTER TABLE credentials ALTER COLUMN team_id SET NOT NULL;
ALTER TABLE credentials ALTER COLUMN team_id SET DEFAULT '00000000-0000-0000-0000-000000000001';

ALTER TABLE workflow_triggers ADD COLUMN team_id UUID REFERENCES teams(id);
UPDATE workflow_triggers SET team_id = '00000000-0000-0000-0000-000000000001' WHERE team_id IS NULL;
ALTER TABLE workflow_triggers ALTER COLUMN team_id SET NOT NULL;
ALTER TABLE workflow_triggers ALTER COLUMN team_id SET DEFAULT '00000000-0000-0000-0000-000000000001';

-- Update unique constraints to be team-scoped
ALTER TABLE workflow_definitions DROP CONSTRAINT workflow_definitions_name_version_key;
ALTER TABLE workflow_definitions ADD CONSTRAINT workflow_definitions_team_name_version_key UNIQUE(team_id, name, version);

ALTER TABLE credentials DROP CONSTRAINT credentials_name_key;
ALTER TABLE credentials ADD CONSTRAINT credentials_team_name_key UNIQUE(team_id, name);

-- Indexes for team-scoped queries
CREATE INDEX idx_workflow_definitions_team ON workflow_definitions(team_id);
CREATE INDEX idx_workflow_executions_team ON workflow_executions(team_id);
CREATE INDEX idx_credentials_team ON credentials(team_id);
CREATE INDEX idx_users_team ON users(team_id);

-- +goose Down
DROP INDEX IF EXISTS idx_users_team;
DROP INDEX IF EXISTS idx_credentials_team;
DROP INDEX IF EXISTS idx_workflow_executions_team;
DROP INDEX IF EXISTS idx_workflow_definitions_team;

ALTER TABLE credentials DROP CONSTRAINT IF EXISTS credentials_team_name_key;
ALTER TABLE credentials ADD CONSTRAINT credentials_name_key UNIQUE(name);

ALTER TABLE workflow_definitions DROP CONSTRAINT IF EXISTS workflow_definitions_team_name_version_key;
ALTER TABLE workflow_definitions ADD CONSTRAINT workflow_definitions_name_version_key UNIQUE(name, version);

ALTER TABLE workflow_triggers DROP COLUMN IF EXISTS team_id;
ALTER TABLE credentials DROP COLUMN IF EXISTS team_id;
ALTER TABLE workflow_executions DROP COLUMN IF EXISTS team_id;
ALTER TABLE workflow_definitions DROP COLUMN IF EXISTS team_id;

DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS teams;
