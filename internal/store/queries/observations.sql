-- name: ListSpeciesAvibaseIDs :many
-- Keyed by avibase_id so a caller can build an id-lookup map; the NULL
-- filter is defensive since NULL can't be a map key anyway.
SELECT id, avibase_id FROM species WHERE avibase_id IS NOT NULL;

-- name: ListHotspotRefsBySource :many
-- Every hotspot's external ref for one source, for building an in-memory
-- lookup before an EBD load (locality id -> hotspot id).
SELECT id, source_ref FROM hotspots WHERE source = sqlc.arg(source);

-- name: UpsertObservation :one
INSERT INTO observations (
    species_id, hotspot_id, location, observed_on, count, source, source_ref
) VALUES (
    sqlc.arg(species_id),
    sqlc.narg(hotspot_id),
    ST_SetSRID(ST_MakePoint(sqlc.arg(longitude), sqlc.arg(latitude)), 4326)::geography,
    sqlc.arg(observed_on),
    sqlc.narg(count),
    sqlc.arg(source),
    sqlc.arg(source_ref)
)
ON CONFLICT (source, source_ref) DO UPDATE SET
    species_id  = EXCLUDED.species_id,
    hotspot_id  = EXCLUDED.hotspot_id,
    location    = EXCLUDED.location,
    observed_on = EXCLUDED.observed_on,
    count       = EXCLUDED.count
RETURNING id;
