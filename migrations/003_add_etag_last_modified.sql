-- +goose Up
-- +goose StatementBegin
ALTER TABLE feeds ADD COLUMN etag TEXT DEFAULT '';
ALTER TABLE feeds ADD COLUMN last_modified TEXT DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE feeds DROP COLUMN last_modified;
ALTER TABLE feeds DROP COLUMN etag;
-- +goose StatementEnd
