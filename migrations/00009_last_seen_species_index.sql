-- +goose Up

-- The species picker asks whether a species has a row at any hotspot. The
-- primary key leads with hotspot_id, so that question needs its own index.
CREATE INDEX hotspot_species_last_seen_species_idx ON hotspot_species_last_seen (species_id);

-- +goose Down

DROP INDEX hotspot_species_last_seen_species_idx;
