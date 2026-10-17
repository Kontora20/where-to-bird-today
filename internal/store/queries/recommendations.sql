-- name: RecommendHotspots :many
-- Ranks hotspots within radius_meters of a point by expected species
-- richness for one ISO week, using the species_seasonality baseline. Species
-- and hotspots are ordered by adjusted_frequency, which discounts frequencies
-- resting on few checklists; frequency and sample_size come back as recorded,
-- so the caller can show what the ranking rests on. Returns
-- one row per (hotspot, top-N species) pair, flattened -- Go groups them back
-- up by hotspot_id. A hotspot with no seasonality rows for that week still
-- appears (score 0, species_id NULL): ST_DWithin found it, so it stays honest
-- to list it rather than silently dropping it. With week_radius above 0 the
-- ranking pools the week with that many weeks either side, which gives a
-- small hotspot more checklists to rest on. When species_ids is given the
-- ranking is for those species only, so a hotspot with none of them that
-- week is left out instead of listed at the bottom with a score of 0.
WITH nearby AS (
    SELECT
        h.id,
        h.name,
        h.source,
        h.source_ref,
        h.region,
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
window_weeks AS (
    -- The week asked for and week_radius weeks either side, the year being a
    -- circle of 52 weeks. A radius of 0 is the week alone.
    SELECT ((sqlc.arg(week_of_year)::int - 1 + d + 52) % 52 + 1)::smallint AS week_of_year
    FROM generate_series(-sqlc.arg(week_radius)::int, sqlc.arg(week_radius)::int) AS d
),
hotspot_checklists AS (
    -- Every species row of a hotspot's week carries the same sample_size, so
    -- the hotspot's checklists of a week are read off any of them. Not limited
    -- to the species asked for: they are what a frequency is a share of.
    SELECT week_totals.hotspot_id, SUM(week_totals.checklists) AS checklists
    FROM (
        SELECT s.hotspot_id, s.week_of_year, MAX(s.sample_size) AS checklists
        FROM species_seasonality s
        JOIN nearby n ON n.id = s.hotspot_id
        WHERE s.week_of_year IN (SELECT week_of_year FROM window_weeks)
        GROUP BY s.hotspot_id, s.week_of_year
    ) week_totals
    GROUP BY week_totals.hotspot_id
),
pooled AS (
    -- Joins nearby in before filtering, not after: species_seasonality's PK
    -- leads with hotspot_id, so restricting to the radius-bounded nearby set
    -- first lets Postgres index-scan per hotspot instead of scanning the
    -- whole table by week_of_year. The checklists that recorded a species are
    -- frequency * sample_size, rounded back to a whole number.
    SELECT s.hotspot_id, s.species_id, SUM(ROUND(s.frequency * s.sample_size)) AS with_species
    FROM species_seasonality s
    JOIN nearby n ON n.id = s.hotspot_id
    WHERE s.week_of_year IN (SELECT week_of_year FROM window_weeks)
      AND (sqlc.narg(species_ids)::bigint[] IS NULL OR s.species_id = ANY(sqlc.narg(species_ids)::bigint[]))
    GROUP BY s.hotspot_id, s.species_id
),
matching AS (
    -- The adjusted frequency is (with_species + prior_checklists *
    -- country_frequency) / (checklists + prior_checklists), the same
    -- adjustment species_seasonality.adjusted_frequency holds for one week,
    -- here over the pooled weeks: the country's frequency is the average of the
    -- weeks, and counts as prior_checklists extra checklists.
    SELECT p.hotspot_id, p.species_id,
           (p.with_species / hc.checklists)::real AS frequency,
           hc.checklists::integer AS sample_size,
           ((p.with_species + sqlc.arg(prior_checklists)::real * country.frequency)
               / (hc.checklists + sqlc.arg(prior_checklists)::real))::real AS adjusted_frequency
    FROM pooled p
    JOIN hotspot_checklists hc ON hc.hotspot_id = p.hotspot_id
    JOIN nearby n ON n.id = p.hotspot_id
    CROSS JOIN LATERAL (
        SELECT COALESCE(SUM(c.frequency), 0) / (2 * sqlc.arg(week_radius)::int + 1) AS frequency
        FROM window_weeks w
        LEFT JOIN country_seasonality c
            ON c.country = n.region AND c.species_id = p.species_id AND c.week_of_year = w.week_of_year
    ) country
),
ranked AS (
    SELECT hotspot_id, species_id, frequency, sample_size,
           ROW_NUMBER() OVER (PARTITION BY hotspot_id ORDER BY adjusted_frequency DESC) AS rank
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
    r.frequency,
    r.sample_size AS checklists
FROM nearby n
LEFT JOIN (
    SELECT hotspot_id, SUM(adjusted_frequency) AS total_frequency FROM matching GROUP BY hotspot_id
) scores ON scores.hotspot_id = n.id
LEFT JOIN ranked r ON r.hotspot_id = n.id AND r.rank <= sqlc.arg(top_species_limit)
LEFT JOIN species sp ON sp.id = r.species_id
WHERE sqlc.narg(species_ids)::bigint[] IS NULL OR scores.total_frequency IS NOT NULL
ORDER BY score DESC, n.id, r.rank;
