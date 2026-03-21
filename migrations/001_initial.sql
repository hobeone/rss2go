-- +goose Up
-- +goose StatementBegin
CREATE TABLE feeds (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    url TEXT NOT NULL UNIQUE,
    title TEXT NOT NULL,
    last_poll DATETIME,
    last_error_time DATETIME,
    last_error_code INTEGER,
    last_error_snippet TEXT
);

CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    email TEXT NOT NULL UNIQUE
);

CREATE TABLE subscriptions (
    user_id INTEGER NOT NULL,
    feed_id INTEGER NOT NULL,
    PRIMARY KEY (user_id, feed_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (feed_id) REFERENCES feeds(id) ON DELETE CASCADE
);

CREATE TABLE seen_items (
    feed_id INTEGER NOT NULL,
    guid TEXT NOT NULL,
    seen_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (feed_id, guid),
    FOREIGN KEY (feed_id) REFERENCES feeds(id) ON DELETE CASCADE
);

CREATE INDEX idx_seen_items_feed_guid ON seen_items(feed_id, guid);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE seen_items;
DROP TABLE subscriptions;
DROP TABLE users;
DROP TABLE feeds;
-- +goose StatementEnd
