-- +goose Up

-- Tracks cumulative AI token usage per team/provider/model/period.
-- One row per (team, provider, model, period_start) — atomically incremented.
CREATE TABLE ai_token_usage (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id         UUID NOT NULL REFERENCES teams(id) ON DELETE RESTRICT,
    provider        TEXT NOT NULL,           -- "openai", "bedrock"
    model           TEXT NOT NULL,           -- "gpt-4o", "claude-3-sonnet", etc.
    period_start    DATE NOT NULL,           -- bucket start date
    prompt_tokens   BIGINT NOT NULL DEFAULT 0,
    completion_tokens BIGINT NOT NULL DEFAULT 0,
    total_tokens    BIGINT NOT NULL DEFAULT 0,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(team_id, provider, model, period_start)
);

CREATE INDEX idx_ai_token_usage_team_period ON ai_token_usage(team_id, period_start);
CREATE INDEX idx_ai_token_usage_team_provider_period ON ai_token_usage(team_id, provider, period_start);

-- Per-team, per-provider budget configuration.
-- If no row exists for a team+provider, the global default applies.
CREATE TABLE team_budgets (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id         UUID NOT NULL REFERENCES teams(id) ON DELETE RESTRICT,
    provider        TEXT NOT NULL,           -- "openai", "bedrock", or "*" for all providers
    monthly_token_limit BIGINT NOT NULL,     -- 0 = unlimited
    enforcement     TEXT NOT NULL DEFAULT 'hard' CHECK (enforcement IN ('hard', 'warn')), -- "hard" (block) or "warn" (log + continue)
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(team_id, provider)
);

-- +goose Down
DROP TABLE IF EXISTS team_budgets;
DROP TABLE IF EXISTS ai_token_usage;
