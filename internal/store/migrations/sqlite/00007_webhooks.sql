-- +goose Up
-- Outgoing webhooks of a user, and their delivery queue (see ADR 0003).
CREATE TABLE webhooks (
    id              TEXT    PRIMARY KEY,
    user_id         TEXT    NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name            TEXT    NOT NULL,
    url             TEXT    NOT NULL,
    format          TEXT    NOT NULL CHECK (format IN ('acksy', 'discord', 'slack')),
    secret          TEXT    NOT NULL,
    min_priority    INTEGER NOT NULL DEFAULT 1 CHECK (min_priority BETWEEN 1 AND 5),
    disabled_at     INTEGER,
    created_at      INTEGER  NOT NULL,
    updated_at      INTEGER  NOT NULL,
    last_success_at INTEGER,
    last_failure_at INTEGER,
    last_error      TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX webhooks_user_idx ON webhooks (user_id);

CREATE TABLE webhook_topics (
    webhook_id TEXT NOT NULL REFERENCES webhooks (id) ON DELETE CASCADE,
    topic_id   TEXT NOT NULL REFERENCES topics (id) ON DELETE CASCADE,
    PRIMARY KEY (webhook_id, topic_id)
);

-- A row is pending while next_attempt_at is set; it is deleted on success
-- and keeps failed_at once retries are exhausted (pruned later).
CREATE TABLE webhook_deliveries (
    id              TEXT    PRIMARY KEY,
    webhook_id      TEXT    NOT NULL REFERENCES webhooks (id) ON DELETE CASCADE,
    event           TEXT    NOT NULL,
    payload         TEXT    NOT NULL, -- notification JSON at publish time
    attempts        INTEGER NOT NULL DEFAULT 0,
    next_attempt_at INTEGER,
    last_status     INTEGER NOT NULL DEFAULT 0,
    last_error      TEXT    NOT NULL DEFAULT '',
    created_at      INTEGER  NOT NULL,
    failed_at       INTEGER
);
CREATE INDEX webhook_deliveries_due_idx ON webhook_deliveries (next_attempt_at) WHERE next_attempt_at IS NOT NULL;
CREATE INDEX webhook_deliveries_webhook_idx ON webhook_deliveries (webhook_id, created_at);

-- +goose Down
DROP TABLE webhook_deliveries;
DROP TABLE webhook_topics;
DROP TABLE webhooks;
