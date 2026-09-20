-- name: GetSpecies :one
SELECT * FROM species WHERE id = sqlc.arg(id);

-- name: SearchSpecies :many
-- Species whose common name (any language) or scientific name contains the
-- query, for the target-species picker. Only species with a hotspot
-- observation are offered: a bird the app holds no data for cannot rank
-- anything. strpos on lowered text, not ILIKE, so a % or _ typed by the user
-- is a plain character rather than a pattern.
SELECT
    sp.id AS species_id,
    sp.scientific_name,
    COALESCE(best.common_name, sp.scientific_name)::text AS common_name
FROM species sp
LEFT JOIN LATERAL (
    SELECT n.common_name
    FROM species_names n
    WHERE n.species_id = sp.id AND n.is_primary
    ORDER BY array_position(sqlc.arg(lang_priority)::text[], n.lang) ASC NULLS LAST
    LIMIT 1
) best ON true
WHERE (
        strpos(lower(sp.scientific_name), lower(sqlc.arg(query)::text)) > 0
        OR EXISTS (
            SELECT 1 FROM species_names m
            WHERE m.species_id = sp.id
              AND strpos(lower(m.common_name), lower(sqlc.arg(query)::text)) > 0
        )
    )
  AND EXISTS (
        SELECT 1 FROM observations o
        WHERE o.species_id = sp.id AND o.hotspot_id IS NOT NULL
    )
ORDER BY common_name
LIMIT sqlc.arg(result_limit);

-- name: UpsertSpecies :one
INSERT INTO species (
    ebird_species_code, scientific_name, avibase_id, iucn_category,
    family, taxon_order, birds_of_the_world_url, birdlife_datazone_url
) VALUES (
    sqlc.narg(ebird_species_code),
    sqlc.arg(scientific_name),
    sqlc.arg(avibase_id),
    sqlc.narg(iucn_category),
    sqlc.narg(family),
    sqlc.narg(taxon_order),
    sqlc.narg(birds_of_the_world_url),
    sqlc.narg(birdlife_datazone_url)
)
ON CONFLICT (avibase_id) DO UPDATE SET
    ebird_species_code     = EXCLUDED.ebird_species_code,
    scientific_name        = EXCLUDED.scientific_name,
    iucn_category           = EXCLUDED.iucn_category,
    family                  = EXCLUDED.family,
    taxon_order             = EXCLUDED.taxon_order,
    birds_of_the_world_url  = EXCLUDED.birds_of_the_world_url,
    birdlife_datazone_url   = EXCLUDED.birdlife_datazone_url,
    updated_at              = now()
RETURNING *;
