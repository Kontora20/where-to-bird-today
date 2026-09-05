-- name: ListRelevantSpecies :many
-- Only species actually observed in the Baltics need photos -- fetching for
-- all of AviList's ~11,000 species when a recommendation can only ever show
-- a few hundred of them would be waste, not thoroughness.
SELECT s.* FROM species s WHERE s.id IN (SELECT DISTINCT species_id FROM observations);

-- name: ListSpeciesMedia :many
-- Every stored photo for one species, best first -- what the species detail
-- panel reads (ADR 0004).
SELECT * FROM species_media WHERE species_id = sqlc.arg(species_id) ORDER BY rank;

-- name: UpsertSpeciesMedia :exec
INSERT INTO species_media (
    species_id, source, source_ref, image_url, thumb_url, width, height,
    license, license_url, attribution, attribution_url, rank
) VALUES (
    sqlc.arg(species_id), sqlc.arg(source), sqlc.arg(source_ref),
    sqlc.arg(image_url), sqlc.narg(thumb_url), sqlc.narg(width), sqlc.narg(height),
    sqlc.arg(license), sqlc.narg(license_url), sqlc.arg(attribution), sqlc.narg(attribution_url),
    sqlc.arg(rank)
)
ON CONFLICT (species_id, source, source_ref) DO UPDATE SET
    image_url       = EXCLUDED.image_url,
    thumb_url       = EXCLUDED.thumb_url,
    width           = EXCLUDED.width,
    height          = EXCLUDED.height,
    license         = EXCLUDED.license,
    license_url     = EXCLUDED.license_url,
    attribution     = EXCLUDED.attribution,
    attribution_url = EXCLUDED.attribution_url,
    rank            = EXCLUDED.rank,
    fetched_at      = now();
