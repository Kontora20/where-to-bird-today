-- +goose Up

-- A species' frequency across a whole country in one week: the share of all
-- complete checklists at the country's hotspots that week that recorded it.
-- It is what a hotspot's weekly frequency is pulled toward when weeks are
-- pooled for the best weeks of a species. A row exists only for a week in
-- which the species was recorded in the country; no row means 0. country is
-- hotspots.region. Filled in by compute-seasonality after species_seasonality.
CREATE TABLE country_seasonality (
    country       text NOT NULL,
    species_id    bigint NOT NULL REFERENCES species(id) ON DELETE CASCADE,
    week_of_year  smallint NOT NULL CHECK (week_of_year BETWEEN 1 AND 52),
    frequency     real NOT NULL,
    PRIMARY KEY (country, species_id, week_of_year)
);

-- +goose Down

DROP TABLE country_seasonality;
