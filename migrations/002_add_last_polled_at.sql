-- +goose Up
-- +goose StatementBegin
ALTER TABLE feeds ADD COLUMN last_polled_at DATETIME;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- SQLite doesn't support dropping columns easily; we can leave this as no-op.
-- +goose StatementEnd
