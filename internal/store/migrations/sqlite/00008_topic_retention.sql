-- +goose Up
-- Per-topic retention overrides: read notifications older than
-- retention_days are deleted, and each recipient keeps at most
-- max_notifications of the topic. NULL inherits the instance defaults.
ALTER TABLE topics ADD COLUMN retention_days INTEGER CHECK (retention_days BETWEEN 1 AND 3650);
ALTER TABLE topics ADD COLUMN max_notifications INTEGER CHECK (max_notifications BETWEEN 10 AND 1000000);

-- +goose Down
ALTER TABLE topics DROP COLUMN max_notifications;
ALTER TABLE topics DROP COLUMN retention_days;
