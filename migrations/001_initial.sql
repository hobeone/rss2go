-- +goose Up
-- +goose StatementBegin
PRAGMA foreign_keys = ON;

CREATE TABLE feeds (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    url TEXT NOT NULL UNIQUE,
    etag TEXT NOT NULL DEFAULT '',
    last_modified TEXT NOT NULL DEFAULT '',
    next_poll_at DATETIME NOT NULL,
    poll_interval_secs INTEGER NOT NULL DEFAULT 3600,
    backoff_factor REAL NOT NULL DEFAULT 1.0,
    last_error_str TEXT NOT NULL DEFAULT '',
    last_error_time DATETIME,
    last_error_snippet TEXT NOT NULL DEFAULT '',
    extract_full_article INTEGER NOT NULL DEFAULT 0, -- 0 for false, 1 for true
    extraction_strategy TEXT NOT NULL DEFAULT 'summary',
    css_selector TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_feeds_next_poll_at ON feeds(next_poll_at);

CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    email TEXT NOT NULL UNIQUE,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE subscriptions (
    user_id INTEGER NOT NULL,
    feed_id INTEGER NOT NULL,
    PRIMARY KEY (user_id, feed_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (feed_id) REFERENCES feeds(id) ON DELETE CASCADE
);

CREATE INDEX idx_subscriptions_feed_id ON subscriptions(feed_id);

CREATE TABLE seen_items (
    feed_id INTEGER NOT NULL,
    guid TEXT NOT NULL,
    seen_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (feed_id, guid),
    FOREIGN KEY (feed_id) REFERENCES feeds(id) ON DELETE CASCADE
);

CREATE TABLE outbox (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    subject TEXT NOT NULL,
    body TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    retry_count INTEGER NOT NULL DEFAULT 0,
    next_attempt_at DATETIME NOT NULL,
    last_attempt_at DATETIME,
    last_error TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_outbox_status_next_attempt ON outbox(status, next_attempt_at);

CREATE TABLE outbox_recipients (
    outbox_id INTEGER NOT NULL,
    email TEXT NOT NULL,
    PRIMARY KEY (outbox_id, email),
    FOREIGN KEY (outbox_id) REFERENCES outbox(id) ON DELETE CASCADE
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS outbox_recipients;
DROP INDEX IF EXISTS idx_outbox_status_next_attempt;
DROP TABLE IF EXISTS outbox;
DROP TABLE IF EXISTS seen_items;
DROP INDEX IF EXISTS idx_subscriptions_feed_id;
DROP TABLE IF EXISTS subscriptions;
DROP TABLE IF EXISTS users;
DROP INDEX IF EXISTS idx_feeds_next_poll_at;
DROP TABLE IF EXISTS feeds;
-- +goose StatementEnd
