-- +goose Up
CREATE UNIQUE INDEX guid_feed_unique_index ON feed_item (feed_info_id,guid);

-- +goose Down
DROP INDEX guid_feed_unique_index;
