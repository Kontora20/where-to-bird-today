-- +goose Up

-- The most recent record of a species at a hotspot, from a complete checklist
-- or not: last seen is about the bird, not the sampling effort. No row means
-- the species was never recorded there. sampling_event_id links to the eBird
-- checklist behind it. Derived from observations so that requests never read
-- that table. Filled in by compute-seasonality.
CREATE TABLE hotspot_species_last_seen (
    hotspot_id         bigint NOT NULL REFERENCES hotspots(id) ON DELETE CASCADE,
    species_id         bigint NOT NULL REFERENCES species(id) ON DELETE CASCADE,
    observed_on        date NOT NULL,
    sampling_event_id  text NOT NULL,
    PRIMARY KEY (hotspot_id, species_id)
);

-- +goose Down

DROP TABLE hotspot_species_last_seen;
