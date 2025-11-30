-- +goose Up
ALTER TABLE "feed_info" ADD COLUMN last_error_response text NOT NULL DEFAULT '';

-- +goose Down
-- SQLite doesn't support DROP COLUMN in older versions, and even in new ones it's restricted.
-- But for completeness:
ALTER TABLE "feed_info" DROP COLUMN last_error_response;
