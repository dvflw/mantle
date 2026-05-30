-- +goose Up
-- 021_git_repo_webhook_secret: adds the webhook_secret column to git_repos.
-- Stores the HMAC-SHA256 shared secret used to verify inbound push webhooks
-- from git providers (GitHub, GitLab, Gitea). NULL means no HMAC verification.
-- The value is sensitive: the CLI never renders it and audit metadata omits it.
ALTER TABLE git_repos
    ADD COLUMN webhook_secret TEXT;

-- +goose Down
ALTER TABLE git_repos DROP COLUMN IF EXISTS webhook_secret;
