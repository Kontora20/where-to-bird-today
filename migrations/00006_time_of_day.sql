-- +goose Up

-- How long after that day's sunrise at the hotspot the checklist was started,
-- in whole minutes. It wraps at 24 hours, so the night is one stretch: a start
-- at 00:10 and one at 23:50 fall next to each other. NULL for checklists with
-- no start time or no hotspot. Filled in by compute-seasonality.
ALTER TABLE checklists ADD COLUMN minutes_after_sunrise smallint
    CHECK (minutes_after_sunrise BETWEEN 0 AND 1439);

-- Counts behind the time-of-day profile, per hour after sunrise. Counts, not
-- frequencies, so a hotspot's figure can be pulled toward the region's when
-- the profile is read, including for hours where the hotspot has no checklists.
-- Each counts a checklist once whoever submitted a copy of it, and only
-- complete checklists at hotspots.
CREATE TABLE time_of_day_totals (
    hotspot_id  bigint NOT NULL REFERENCES hotspots(id) ON DELETE CASCADE,
    hour_bin    smallint NOT NULL CHECK (hour_bin BETWEEN 0 AND 23),
    checklists  integer NOT NULL,
    PRIMARY KEY (hotspot_id, hour_bin)
);

-- The checklists among those that recorded the species. No row means none.
CREATE TABLE species_time_of_day (
    hotspot_id  bigint NOT NULL REFERENCES hotspots(id) ON DELETE CASCADE,
    species_id  bigint NOT NULL REFERENCES species(id) ON DELETE CASCADE,
    hour_bin    smallint NOT NULL CHECK (hour_bin BETWEEN 0 AND 23),
    checklists  integer NOT NULL,
    PRIMARY KEY (hotspot_id, species_id, hour_bin)
);

-- The region's figure for a species adds up its rows across hotspots.
CREATE INDEX species_time_of_day_species_idx ON species_time_of_day (species_id);

-- +goose Down

DROP TABLE species_time_of_day;
DROP TABLE time_of_day_totals;
ALTER TABLE checklists DROP COLUMN minutes_after_sunrise;
