-- +goose Up
CREATE TABLE environments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) DEFAULT '00000000-0000-0000-0000-000000000001',
    name TEXT NOT NULL,
    inputs JSONB,
    env JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT environments_team_name_key UNIQUE(team_id, name)
);

CREATE INDEX idx_environments_team ON environments(team_id);

-- +goose Down
DROP TABLE IF EXISTS environments;
