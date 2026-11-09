-- +goose Up
-- Web Push subscriptions: one row per browser (endpoint) and user.
CREATE TABLE push_subscriptions (
    id              TEXT    PRIMARY KEY,
    user_id         TEXT    NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    endpoint        TEXT    NOT NULL UNIQUE,
    p256dh          TEXT    NOT NULL,
    auth            TEXT    NOT NULL,
    name            TEXT    NOT NULL DEFAULT '',
    min_priority    INTEGER NOT NULL DEFAULT 3 CHECK (min_priority BETWEEN 1 AND 5),
    created_at      INTEGER  NOT NULL,
    last_success_at INTEGER,
    failures        INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX push_subscriptions_user_idx ON push_subscriptions (user_id);

-- +goose Down
DROP TABLE push_subscriptions;
