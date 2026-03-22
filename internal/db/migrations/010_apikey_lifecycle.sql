-- +goose Up
ALTER TABLE api_keys ADD COLUMN expires_at TIMESTAMPTZ;
ALTER TABLE api_keys ADD COLUMN revoked_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE api_keys DROP COLUMN IF EXISTS revoked_at;
ALTER TABLE api_keys DROP COLUMN IF EXISTS expires_at;
