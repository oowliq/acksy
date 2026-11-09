-- +goose Up
-- Lowercased title, body and tags for substring search (see ADR 0005).
-- Filled by the store on write and by migration 10 for existing rows.
ALTER TABLE notifications ADD COLUMN search_text TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE notifications DROP COLUMN search_text;
