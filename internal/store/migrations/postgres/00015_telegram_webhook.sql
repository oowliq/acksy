-- +goose Up
ALTER TABLE webhooks DROP CONSTRAINT webhooks_format_check;
ALTER TABLE webhooks ADD CONSTRAINT webhooks_format_check CHECK (format IN ('acksy', 'discord', 'slack', 'telegram'));

-- +goose Down
-- Remove Telegram webhooks before downgrading.
ALTER TABLE webhooks DROP CONSTRAINT webhooks_format_check;
ALTER TABLE webhooks ADD CONSTRAINT webhooks_format_check CHECK (format IN ('acksy', 'discord', 'slack'));
