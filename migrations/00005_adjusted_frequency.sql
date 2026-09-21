-- +goose Up

-- frequency pulled toward the country's frequency for the same species and
-- week, by how few checklists it rests on. Hotspots are ranked by this column;
-- frequency stays as eBird would show it. Existing rows get the unadjusted
-- value until compute-seasonality is run again.
ALTER TABLE species_seasonality ADD COLUMN adjusted_frequency real NOT NULL DEFAULT 0;
UPDATE species_seasonality SET adjusted_frequency = frequency;
ALTER TABLE species_seasonality ALTER COLUMN adjusted_frequency DROP DEFAULT;

-- +goose Down

ALTER TABLE species_seasonality DROP COLUMN adjusted_frequency;
