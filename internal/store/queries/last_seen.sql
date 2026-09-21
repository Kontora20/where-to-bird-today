-- name: TruncateLastSeen :exec
TRUNCATE hotspot_species_last_seen;

-- name: RecomputeLastSeen :execrows
-- The latest record of each species at each hotspot. Records with no checklist
-- (loaded before checklists were kept) have no link to give and are left out.
-- Two checklists on the same day are ordered by id, so the result is the same
-- on every run.
INSERT INTO hotspot_species_last_seen (hotspot_id, species_id, observed_on, sampling_event_id)
SELECT DISTINCT ON (o.hotspot_id, o.species_id) o.hotspot_id, o.species_id, o.observed_on, c.sampling_event_id
FROM observations o
JOIN checklists c ON c.id = o.checklist_id
WHERE o.hotspot_id IS NOT NULL
ORDER BY o.hotspot_id, o.species_id, o.observed_on DESC, c.sampling_event_id;

-- name: GetLastSeen :many
-- When each of the species was last recorded at each of the hotspots. A pair
-- with no row was never recorded. The latest record of a hotspot comes first.
SELECT hotspot_id, species_id, observed_on, sampling_event_id
FROM hotspot_species_last_seen
WHERE hotspot_id = ANY(sqlc.arg(hotspot_ids)::bigint[])
  AND species_id = ANY(sqlc.arg(species_ids)::bigint[])
ORDER BY hotspot_id, observed_on DESC, species_id;
