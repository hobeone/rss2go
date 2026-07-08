-- +goose Up
-- +goose StatementBegin
UPDATE feeds SET extraction_strategy = 'selector' WHERE extraction_strategy = 'css';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
UPDATE feeds SET extraction_strategy = 'css' WHERE extraction_strategy = 'selector';
-- +goose StatementEnd
