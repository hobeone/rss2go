-- +goose Up
ALTER TABLE feeds ADD COLUMN extraction_strategy TEXT NOT NULL DEFAULT 'readability';
ALTER TABLE feeds ADD COLUMN extraction_config TEXT;

-- +goose Down
ALTER TABLE feeds DROP COLUMN extraction_config;
ALTER TABLE feeds DROP COLUMN extraction_strategy;
