-- +goose Up
-- Finds every recipient's copy of a message by topic and dedup key (ntfy
-- sequence IDs: update, delete, clear).
CREATE INDEX notifications_topic_dedup_idx ON notifications (topic_id, dedup_key) WHERE dedup_key IS NOT NULL;

-- +goose Down
DROP INDEX notifications_topic_dedup_idx;
