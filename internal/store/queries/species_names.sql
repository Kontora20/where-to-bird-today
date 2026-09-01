-- name: UpsertSpeciesName :execrows
INSERT INTO species_names (species_id, lang, common_name, source, is_primary)
SELECT id, sqlc.arg(lang), sqlc.arg(common_name), sqlc.arg(source), sqlc.arg(is_primary)
FROM species
WHERE ebird_species_code = sqlc.arg(ebird_species_code)
ON CONFLICT (species_id, lang, source) DO UPDATE SET
    common_name = EXCLUDED.common_name,
    is_primary  = EXCLUDED.is_primary;
