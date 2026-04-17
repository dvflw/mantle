-- +goose Up
-- environments stores named reusable input and env-var override sets for
-- parameterized workflow runs ("production", "staging", etc.), resolved by
-- `mantle run --env <name>` and `mantle plan --env <name>`. See issue #53.
--
-- The inputs and env JSONB columns are intentionally unconstrained so
-- workflow authors can store arbitrary structured overrides. Raw env
-- values are never exposed by the CLI without an explicit --reveal flag,
-- and every reveal emits an environment.revealed audit event.
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
