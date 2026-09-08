-- name: UpsertSpeciesName :execrows
INSERT INTO species_names (species_id, lang, common_name, source, is_primary)
SELECT id, sqlc.arg(lang), sqlc.arg(common_name), sqlc.arg(source), sqlc.arg(is_primary)
FROM species
WHERE ebird_species_code = sqlc.arg(ebird_species_code)
ON CONFLICT (species_id, lang, source) DO UPDATE SET
    common_name = EXCLUDED.common_name,
    is_primary  = EXCLUDED.is_primary;

-- name: GetCommonNames :many
-- Resolves one name per species, already fallback-ranked by lang_priority
-- (e.g. ['en','lt']) -- the caller never sees per-language rows to pick
-- between. array_position ranks each row by where its lang sits in the
-- priority list; NULLS LAST pushes any row in a language outside that list
-- to the bottom; DISTINCT ON keeps only the best-ranked row per species.
-- This generalizes to any future language list with no query change.
SELECT DISTINCT ON (species_id)
    species_id,
    common_name
FROM species_names
WHERE species_id = ANY(sqlc.arg(species_ids)::bigint[]) AND is_primary = true
ORDER BY species_id, array_position(sqlc.arg(lang_priority)::text[], lang) ASC NULLS LAST;
