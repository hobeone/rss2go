-- +goose Up
CREATE TABLE "user_report" (
    "id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    "user_id" bigint NOT NULL,
    "last_report" datetime NOT NULL
);
CREATE UNIQUE INDEX user_report_idx ON user_report (id, user_id);

-- +goose Down
DROP TABLE "user_report";
