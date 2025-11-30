-- +goose Up
ALTER TABLE "feed_info" ADD COLUMN site_url text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE "feed_info" DROP COLUMN site_url;
