-- name: ListSpeciesEbirdCodes :many
-- Every species that has an eBird code, so a loader can turn the codes in an
-- API response into species ids without a query per record.
SELECT id, ebird_species_code FROM species WHERE ebird_species_code IS NOT NULL;

-- name: UpsertLastSeen :execrows
-- Records a sighting as the last one at a hotspot only if it is newer than
-- what is stored, so a re-run, an out-of-order record or an older API answer
-- never moves a date back.
INSERT INTO hotspot_species_last_seen (hotspot_id, species_id, observed_on, sampling_event_id)
VALUES (sqlc.arg(hotspot_id), sqlc.arg(species_id), sqlc.arg(observed_on), sqlc.arg(sampling_event_id))
ON CONFLICT (hotspot_id, species_id) DO UPDATE SET
    observed_on       = EXCLUDED.observed_on,
    sampling_event_id = EXCLUDED.sampling_event_id
WHERE EXCLUDED.observed_on > hotspot_species_last_seen.observed_on;
