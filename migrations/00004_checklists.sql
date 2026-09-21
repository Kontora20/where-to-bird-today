-- +goose Up

-- One row per eBird checklist. sampling_event_id is eBird's S-number, kept so
-- every count links back to ebird.org/checklist/<id>. A checklist shared by a
-- group of birders is submitted once per person, each copy with its own
-- sampling_event_id and all with the same group_id: counting the copies would
-- inflate every frequency, so a checklist is identified by
-- COALESCE(group_id, sampling_event_id).
--
-- hotspot_id is NULL for a personal or incidental location. started_at is
-- local time and NULL when the birder did not record one.
CREATE TABLE checklists (
    id                    bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    sampling_event_id     text NOT NULL UNIQUE,
    group_id              text,
    hotspot_id            bigint REFERENCES hotspots(id),
    observed_on           date NOT NULL,
    started_at            time,
    all_species_reported  boolean NOT NULL
);

CREATE INDEX checklists_hotspot_observed_on_idx ON checklists (hotspot_id, observed_on);

-- Nullable because the rows loaded before this migration have no checklist
-- until the export is loaded again; the seasonality rebuild ignores them.
ALTER TABLE observations ADD COLUMN checklist_id bigint REFERENCES checklists(id);

-- +goose Down

ALTER TABLE observations DROP COLUMN checklist_id;
DROP TABLE checklists;
