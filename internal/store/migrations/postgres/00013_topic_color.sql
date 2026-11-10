-- +goose Up
-- Empty values retain the original ID-based swatch for existing topics.
ALTER TABLE topics ADD COLUMN color TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE topics DROP COLUMN color;
