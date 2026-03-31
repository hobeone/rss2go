-- +goose Up
ALTER TABLE feeds ADD COLUMN backoff_until DATETIME;

-- +goose Down
ALTER TABLE feeds DROP COLUMN backoff_until;
