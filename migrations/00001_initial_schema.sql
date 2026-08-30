-- +goose Up

CREATE EXTENSION IF NOT EXISTS postgis;

-- Birding locations. One row per hotspot, deduplicated per source by
-- (source, source_ref) so a re-run of an ingest upserts instead of
-- duplicating -- source_ref is where an eBird locId lives, so every hotspot
-- keeps a stable link back to the eBird page it came from.
CREATE TABLE hotspots (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name        text NOT NULL,
    location    geography(Point, 4326) NOT NULL,
    region      text NOT NULL,
    source      text NOT NULL,
    source_ref  text NOT NULL,
    metadata    jsonb NOT NULL DEFAULT '{}',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (source, source_ref)
);

-- "Hotspots within N km of a point" is ST_DWithin against this column.
-- Without the GiST index that is a sequential scan over every hotspot.
CREATE INDEX hotspots_location_gix ON hotspots USING GIST (location);

-- The species reference backbone, loaded once from AviList.
-- Everything else in the schema joins to this table's id.
--
-- The column is named taxon_order, not order: ORDER is a reserved SQL
-- keyword, and a reserved-word column name has to be double-quoted in every
-- query that touches it forever.
CREATE TABLE species (
    id                      bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    ebird_species_code      text UNIQUE,
    scientific_name         text NOT NULL,
    avibase_id              text,
    iucn_category           text,
    family                  text,
    taxon_order             text,
    birds_of_the_world_url  text,
    birdlife_datazone_url   text,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now()
);

-- Common names per species and language, source-ranked so the lt -> en ->
-- scientific_name fallback resolves in one SQL query. Empty until
-- Phase 2's eBird taxonomy load;
CREATE TABLE species_names (
    species_id   bigint NOT NULL REFERENCES species(id) ON DELETE CASCADE,
    lang         text NOT NULL,
    common_name  text NOT NULL,
    source       text NOT NULL,
    is_primary   boolean NOT NULL DEFAULT false,
    PRIMARY KEY (species_id, lang, source)
);

-- Photo references per species. Stores URLs and licence metadata
-- only, never image bytes. license and attribution are NOT NULL on purpose:
-- CC BY-SA and iNaturalist both require the credit to render next to the
-- image, so a row without one is not a usable row.
CREATE TABLE species_media (
    species_id        bigint NOT NULL REFERENCES species(id) ON DELETE CASCADE,
    source             text NOT NULL,
    source_ref         text NOT NULL,
    image_url          text NOT NULL,
    thumb_url          text,
    width              integer,
    height             integer,
    license            text NOT NULL,
    license_url        text,
    attribution        text NOT NULL,
    attribution_url    text,
    rank               integer NOT NULL DEFAULT 0,
    fetched_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (species_id, source, source_ref)
);

-- One row per observation, from either the EBD bulk load or a live eBird/iNat
-- sync. hotspot_id is nullable: a personal or incidental location has no
-- hotspot to point at, but still has a location for point queries.
-- source_ref carries the eBird subId, so every observation keeps a link back.
CREATE TABLE observations (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    species_id   bigint NOT NULL REFERENCES species(id),
    hotspot_id   bigint REFERENCES hotspots(id),
    location     geography(Point, 4326) NOT NULL,
    observed_on  date NOT NULL,
    count        integer,
    source       text NOT NULL,
    source_ref   text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (source, source_ref)
);

CREATE INDEX observations_location_gix ON observations USING GIST (location);
-- Both orderings are queried separately: "what's expected at this hotspot"
-- and "what's expected for this species" are different lookups, and a
-- composite index only serves the query that leads with its first column.
CREATE INDEX observations_hotspot_observed_on_idx ON observations (hotspot_id, observed_on);
CREATE INDEX observations_species_observed_on_idx ON observations (species_id, observed_on);

-- The precomputed baseline: per hotspot x species x ISO week-of-year, how
-- often it's actually seen there. This is what "what to expect" is built
-- from -- derived from observations by a Phase 2 batch job, not written to
-- per request.
CREATE TABLE species_seasonality (
    hotspot_id    bigint NOT NULL REFERENCES hotspots(id) ON DELETE CASCADE,
    species_id    bigint NOT NULL REFERENCES species(id) ON DELETE CASCADE,
    week_of_year  smallint NOT NULL CHECK (week_of_year BETWEEN 1 AND 53),
    frequency     real NOT NULL,
    sample_size   integer NOT NULL,
    computed_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (hotspot_id, species_id, week_of_year)
);

-- Bookkeeping for every sync/load
CREATE TABLE ingestion_runs (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source       text NOT NULL,
    started_at   timestamptz NOT NULL DEFAULT now(),
    finished_at  timestamptz,
    row_count    integer,
    status       text NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'succeeded', 'failed')),
    error        text
);

-- +goose Down

DROP TABLE ingestion_runs;
DROP TABLE species_seasonality;
DROP TABLE observations;
DROP TABLE species_media;
DROP TABLE species_names;
DROP TABLE species;
DROP TABLE hotspots;

-- The postgis extension is deliberately left in place: it is a
-- cluster-level capability the postgis image installs at initdb time, not
-- schema this migration owns, and dropping/recreating it on every up/down
-- cycle is slow for no benefit in a dev loop.
