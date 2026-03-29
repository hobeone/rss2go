-- +goose Up
-- +goose StatementBegin
ALTER TABLE feeds ADD COLUMN full_article BOOLEAN DEFAULT FALSE;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE feeds DROP COLUMN full_article;
-- +goose StatementEnd
