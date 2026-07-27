-- +goose Up
-- +goose StatementBegin
ALTER TABLE feeds ADD COLUMN scraper_item_selector TEXT NOT NULL DEFAULT '';
ALTER TABLE feeds ADD COLUMN scraper_title_selector TEXT NOT NULL DEFAULT '';
ALTER TABLE feeds ADD COLUMN scraper_link_selector TEXT NOT NULL DEFAULT '';
ALTER TABLE feeds ADD COLUMN scraper_description_selector TEXT NOT NULL DEFAULT '';

-- Update the existing EC-Stories feed to the native fields
UPDATE feeds
SET 
  url = 'https://escapecollective.com/stories/',
  scraper_item_selector = 'div.post',
  scraper_title_selector = 'h3',
  scraper_link_selector = 'a'
WHERE url = 'http://localhost:8282/scrape?url=https%3A%2F%2Fescapecollective.com%2Fstories%2F&item=div.post&title=h3&link=a';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
UPDATE feeds
SET url = 'http://localhost:8282/scrape?url=https%3A%2F%2Fescapecollective.com%2Fstories%2F&item=div.post&title=h3&link=a'
WHERE url = 'https://escapecollective.com/stories/';

ALTER TABLE feeds DROP COLUMN scraper_item_selector;
ALTER TABLE feeds DROP COLUMN scraper_title_selector;
ALTER TABLE feeds DROP COLUMN scraper_link_selector;
ALTER TABLE feeds DROP COLUMN scraper_description_selector;
-- +goose StatementEnd
