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
