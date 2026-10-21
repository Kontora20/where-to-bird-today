-- name: UpsertChecklist :one
INSERT INTO checklists (
    sampling_event_id, group_id, hotspot_id, observed_on, started_at, all_species_reported
) VALUES (
    sqlc.arg(sampling_event_id),
    sqlc.narg(group_id),
    sqlc.narg(hotspot_id),
    sqlc.arg(observed_on),
    sqlc.narg(started_at),
    sqlc.arg(all_species_reported)
)
ON CONFLICT (sampling_event_id) DO UPDATE SET
    group_id             = EXCLUDED.group_id,
    hotspot_id           = EXCLUDED.hotspot_id,
    observed_on          = EXCLUDED.observed_on,
    started_at           = EXCLUDED.started_at,
    all_species_reported = EXCLUDED.all_species_reported
RETURNING id;

-- name: GetLatestChecklistWithSpecies :one
-- The most recent checklist at a hotspot that recorded a species, so the
-- species panel can link to the eBird page behind the numbers. No row means
-- the species was never recorded there. Read from hotspot_species_last_seen,
-- which compute-seasonality fills.
SELECT sampling_event_id, observed_on
FROM hotspot_species_last_seen
WHERE species_id = sqlc.arg(species_id) AND hotspot_id = sqlc.arg(hotspot_id);

-- name: GetLatestChecklistDate :one
-- The date of the newest checklist loaded at a country's hotspots, where the
-- eBird checklist load of that country resumes from. NULL when there are none.
SELECT MAX(c.observed_on)::date AS latest
FROM checklists c
JOIN hotspots h ON h.id = c.hotspot_id
WHERE h.region = sqlc.arg(region);

-- name: ListChecklistSamplingEventIDsSince :many
-- The checklists already loaded from a date on, so the daily load does not
-- fetch them from eBird again.
SELECT sampling_event_id FROM checklists WHERE observed_on >= sqlc.arg(since);
