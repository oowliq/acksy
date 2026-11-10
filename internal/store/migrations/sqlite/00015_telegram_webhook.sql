-- +goose Up
ALTER TABLE webhooks ADD COLUMN next_format TEXT NOT NULL DEFAULT 'acksy' CHECK (next_format IN ('acksy', 'discord', 'slack', 'telegram'));
UPDATE webhooks SET next_format = format;
ALTER TABLE webhooks DROP COLUMN format;
ALTER TABLE webhooks RENAME COLUMN next_format TO format;

-- +goose Down
-- Remove Telegram webhooks before downgrading.
ALTER TABLE webhooks ADD COLUMN next_format TEXT NOT NULL DEFAULT 'acksy' CHECK (next_format IN ('acksy', 'discord', 'slack'));
UPDATE webhooks SET next_format = format;
ALTER TABLE webhooks DROP COLUMN format;
ALTER TABLE webhooks RENAME COLUMN next_format TO format;
