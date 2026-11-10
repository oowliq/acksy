-- +goose Up
ALTER TABLE api_tokens DROP COLUMN prefix;

-- +goose Down
ALTER TABLE api_tokens ADD COLUMN prefix TEXT NOT NULL DEFAULT '';
