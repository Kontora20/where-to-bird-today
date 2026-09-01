-- +goose Up

-- ebird_species_code is NULL for some species, and NULL never conflicts
-- with itself, so an upsert keyed on it would duplicate those rows on every
-- re-run. avibase_id is always populated, so it is the reliable upsert key.
CREATE UNIQUE INDEX species_avibase_id_key ON species (avibase_id);

-- +goose Down

DROP INDEX species_avibase_id_key;
