-- name: TruncateSpeciesSeasonality :exec
TRUNCATE species_seasonality;

-- name: RecomputeSpeciesSeasonality :execrows
-- Rebuilds the whole table from observations: per hotspot x species x ISO
-- week, distinct days that species was logged there, divided by distinct
-- days anything was logged there. hotspot_id IS NOT NULL excludes personal
-- and incidental locations -- a per-hotspot stat needs a hotspot.
WITH species_week_days AS (
    SELECT hotspot_id, species_id, EXTRACT(week FROM observed_on)::smallint AS week_of_year,
           COUNT(DISTINCT observed_on) AS days
    FROM observations
    WHERE hotspot_id IS NOT NULL
    GROUP BY hotspot_id, species_id, week_of_year
),
hotspot_week_days AS (
    SELECT hotspot_id, EXTRACT(week FROM observed_on)::smallint AS week_of_year,
           COUNT(DISTINCT observed_on) AS days
    FROM observations
    WHERE hotspot_id IS NOT NULL
    GROUP BY hotspot_id, week_of_year
)
INSERT INTO species_seasonality (hotspot_id, species_id, week_of_year, frequency, sample_size)
SELECT s.hotspot_id, s.species_id, s.week_of_year,
       s.days::real / h.days::real,
       h.days
FROM species_week_days s
JOIN hotspot_week_days h USING (hotspot_id, week_of_year);
