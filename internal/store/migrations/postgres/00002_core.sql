-- +goose Up
-- Timestamps are Unix milliseconds (UTC) in both dialects so that the same
-- portable SQL works on SQLite and PostgreSQL. See docs/adr/0001-portable-sql.md.

CREATE TABLE users (
    id            TEXT    PRIMARY KEY,
    username      TEXT    NOT NULL UNIQUE,
    display_name  TEXT    NOT NULL DEFAULT '',
    email         TEXT,
    password_hash TEXT,
    role          TEXT    NOT NULL CHECK (role IN ('admin', 'user')),
    event_seq     BIGINT  NOT NULL DEFAULT 0,
    disabled_at   BIGINT,
    created_at    BIGINT  NOT NULL,
    updated_at    BIGINT  NOT NULL
);

CREATE TABLE sessions (
    id           TEXT   PRIMARY KEY, -- sha256 of the cookie value
    user_id      TEXT   NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    user_agent   TEXT   NOT NULL DEFAULT '',
    created_at   BIGINT NOT NULL,
    expires_at   BIGINT NOT NULL,
    last_seen_at BIGINT NOT NULL
);
CREATE INDEX sessions_user_idx ON sessions (user_id);

CREATE TABLE topics (
    id               TEXT    PRIMARY KEY,
    owner_id         TEXT    NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name             TEXT    NOT NULL,
    title            TEXT    NOT NULL DEFAULT '',
    description      TEXT    NOT NULL DEFAULT '',
    default_priority INTEGER CHECK (default_priority BETWEEN 1 AND 5),
    created_at       BIGINT  NOT NULL,
    updated_at       BIGINT  NOT NULL,
    UNIQUE (owner_id, name)
);

CREATE TABLE topic_members (
    topic_id   TEXT   NOT NULL REFERENCES topics (id) ON DELETE CASCADE,
    user_id    TEXT   NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role       TEXT   NOT NULL CHECK (role IN ('owner', 'member')),
    created_at BIGINT NOT NULL,
    PRIMARY KEY (topic_id, user_id)
);
CREATE INDEX topic_members_user_idx ON topic_members (user_id);

CREATE TABLE api_tokens (
    id           TEXT   PRIMARY KEY,
    user_id      TEXT   NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name         TEXT   NOT NULL,
    prefix       TEXT   NOT NULL,
    secret_hash  TEXT   NOT NULL UNIQUE,
    scopes       TEXT   NOT NULL,
    expires_at   BIGINT,
    last_used_at BIGINT,
    created_at   BIGINT NOT NULL
);
CREATE INDEX api_tokens_user_idx ON api_tokens (user_id);

CREATE TABLE api_token_topics (
    token_id TEXT NOT NULL REFERENCES api_tokens (id) ON DELETE CASCADE,
    topic_id TEXT NOT NULL REFERENCES topics (id) ON DELETE CASCADE,
    PRIMARY KEY (token_id, topic_id)
);

CREATE TABLE notifications (
    id            TEXT    PRIMARY KEY, -- ULID, sortable by creation
    user_id       TEXT    NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    topic_id      TEXT    REFERENCES topics (id) ON DELETE SET NULL,
    message_id    TEXT    NOT NULL,
    title         TEXT    NOT NULL DEFAULT '',
    body          TEXT    NOT NULL DEFAULT '',
    priority      INTEGER NOT NULL CHECK (priority BETWEEN 1 AND 5),
    tags          TEXT    NOT NULL DEFAULT '[]', -- JSON array
    click_url     TEXT    NOT NULL DEFAULT '',
    actions       TEXT    NOT NULL DEFAULT '[]', -- JSON array
    group_key     TEXT,
    dedup_key     TEXT,
    repeat_count  INTEGER NOT NULL DEFAULT 1,
    source        TEXT    NOT NULL,
    token_id      TEXT    REFERENCES api_tokens (id) ON DELETE SET NULL,
    created_at    BIGINT  NOT NULL,
    updated_at    BIGINT  NOT NULL,
    expires_at    BIGINT,
    read_at       BIGINT,
    archived_at   BIGINT,
    snoozed_until BIGINT
);
CREATE INDEX notifications_user_idx ON notifications (user_id, id);
CREATE INDEX notifications_user_topic_idx ON notifications (user_id, topic_id, id);
CREATE INDEX notifications_user_unread_idx ON notifications (user_id, read_at, archived_at);
CREATE INDEX notifications_user_group_idx ON notifications (user_id, group_key) WHERE group_key IS NOT NULL;
CREATE UNIQUE INDEX notifications_dedup_idx ON notifications (user_id, dedup_key) WHERE dedup_key IS NOT NULL;

CREATE TABLE events (
    user_id    TEXT   NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    seq        BIGINT NOT NULL,
    type       TEXT   NOT NULL,
    payload    TEXT   NOT NULL, -- JSON
    created_at BIGINT NOT NULL,
    PRIMARY KEY (user_id, seq)
);

-- +goose Down
DROP TABLE events;
DROP TABLE notifications;
DROP TABLE api_token_topics;
DROP TABLE api_tokens;
DROP TABLE topic_members;
DROP TABLE topics;
DROP TABLE sessions;
DROP TABLE users;
