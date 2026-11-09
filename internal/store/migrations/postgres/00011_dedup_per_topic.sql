-- +goose Up
-- A dedup key identifies a message within its topic (or among the user's
-- topic-less notifications). Unique per user only, a publisher to one topic
-- could overwrite the recipient's notifications in any other topic.
DROP INDEX notifications_dedup_idx;
CREATE UNIQUE INDEX notifications_dedup_idx
    ON notifications (user_id, (COALESCE(topic_id, '')), dedup_key) WHERE dedup_key IS NOT NULL;

-- +goose Down
-- Keeps the newest notification per (user, key) so the old index can be built.
DELETE FROM notifications WHERE dedup_key IS NOT NULL AND EXISTS (
    SELECT 1 FROM notifications newer
    WHERE newer.user_id = notifications.user_id AND newer.dedup_key = notifications.dedup_key
      AND newer.id > notifications.id);
DROP INDEX notifications_dedup_idx;
CREATE UNIQUE INDEX notifications_dedup_idx ON notifications (user_id, dedup_key) WHERE dedup_key IS NOT NULL;
