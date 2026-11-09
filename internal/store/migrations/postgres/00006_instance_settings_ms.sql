-- +goose Up
-- 00001 declared updated_at as TIMESTAMPTZ, unlike every other timestamp
-- (Unix milliseconds, ADR 0001). Convert it so the portable queries work.
ALTER TABLE instance_settings
    ALTER COLUMN updated_at TYPE BIGINT USING (EXTRACT(EPOCH FROM updated_at) * 1000)::BIGINT;

-- +goose Down
ALTER TABLE instance_settings
    ALTER COLUMN updated_at TYPE TIMESTAMPTZ USING to_timestamp(updated_at / 1000.0);
