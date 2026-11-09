-- +goose Up
-- Postgres-only fix (instance_settings.updated_at to Unix milliseconds); the
-- SQLite column is already INTEGER. Kept so both dialects share numbering.
SELECT 1;

-- +goose Down
SELECT 1;
