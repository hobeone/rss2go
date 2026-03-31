-- +goose Up
-- +goose StatementBegin
CREATE TABLE outbox (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    recipients TEXT NOT NULL,
    subject TEXT NOT NULL,
    body TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    delivered_at DATETIME
);

CREATE INDEX idx_outbox_status ON outbox(status, created_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE outbox;
-- +goose StatementEnd
