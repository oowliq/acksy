-- +goose Up
-- Instance-wide key/value settings (generated VAPID keys, bootstrap flags, ...).
CREATE TABLE instance_settings (
    key        TEXT    PRIMARY KEY,
    value      TEXT    NOT NULL,
    updated_at INTEGER NOT NULL
);

-- +goose Down
DROP TABLE instance_settings;
