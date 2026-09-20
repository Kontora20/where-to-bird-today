-- name: RecommendHotspots :many
-- Ranks hotspots within radius_meters of a point by expected species
-- richness for one ISO week, using the species_seasonality baseline. Returns
-- one row per (hotspot, top-N species) pair, flattened -- Go groups them back
-- up by hotspot_id. A hotspot with no seasonality rows for that week still
-- appears (score 0, species_id NULL): ST_DWithin found it, so it stays honest
-- to list it rather than silently dropping it. When species_ids is given the
-- ranking is for those species only, so a hotspot with none of them that
-- week is left out instead of listed at the bottom with a score of 0.
WITH nearby AS (
    SELECT
        h.id,
        h.name,
        h.source,
        h.source_ref,
        ST_X(h.location::geometry) AS longitude,
        ST_Y(h.location::geometry) AS latitude,
        ST_Distance(h.location, ST_SetSRID(ST_MakePoint(sqlc.arg(longitude), sqlc.arg(latitude)), 4326)::geography) AS distance_meters
    FROM hotspots h
    WHERE ST_DWithin(
        h.location,
        ST_SetSRID(ST_MakePoint(sqlc.arg(longitude), sqlc.arg(latitude)), 4326)::geography,
        sqlc.arg(radius_meters)
    )
),
matching AS (
    -- Joins nearby in before filtering, not after: species_seasonality's PK
    -- leads with hotspot_id, so restricting to the radius-bounded nearby set
    -- first lets Postgres index-scan per hotspot instead of scanning the
    -- whole table by week_of_year. This CTE is referenced twice below, which
    -- stops Postgres inlining it, so this join order is what actually runs.
    SELECT s.hotspot_id, s.species_id, s.frequency
    FROM species_seasonality s
    JOIN nearby n ON n.id = s.hotspot_id
    WHERE s.week_of_year = sqlc.arg(week_of_year)
      AND (sqlc.narg(species_ids)::bigint[] IS NULL OR s.species_id = ANY(sqlc.narg(species_ids)::bigint[]))
),
ranked AS (
    SELECT hotspot_id, species_id, frequency,
           ROW_NUMBER() OVER (PARTITION BY hotspot_id ORDER BY frequency DESC) AS rank
    FROM matching
)
SELECT
    n.id AS hotspot_id,
    n.name AS hotspot_name,
    n.source AS hotspot_source,
    n.source_ref AS hotspot_source_ref,
    n.latitude,
    n.longitude,
    n.distance_meters,
    COALESCE(scores.total_frequency, 0)::real AS score,
    r.species_id,
    sp.scientific_name,
    r.frequency
FROM nearby n
LEFT JOIN (
    SELECT hotspot_id, SUM(frequency) AS total_frequency FROM matching GROUP BY hotspot_id
) scores ON scores.hotspot_id = n.id
LEFT JOIN ranked r ON r.hotspot_id = n.id AND r.rank <= sqlc.arg(top_species_limit)
LEFT JOIN species sp ON sp.id = r.species_id
WHERE sqlc.narg(species_ids)::bigint[] IS NULL OR scores.total_frequency IS NOT NULL
ORDER BY score DESC, n.id, r.frequency DESC NULLS LAST;
