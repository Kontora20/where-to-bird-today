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
-- the species was never recorded there.
SELECT c.sampling_event_id, c.observed_on
FROM observations o
JOIN checklists c ON c.id = o.checklist_id
WHERE o.species_id = sqlc.arg(species_id) AND o.hotspot_id = sqlc.arg(hotspot_id)
ORDER BY o.observed_on DESC
LIMIT 1;
