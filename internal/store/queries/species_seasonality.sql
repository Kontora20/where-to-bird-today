-- name: GetSpeciesSeasonalityAtHotspot :many
-- One species' week-by-week frequency at one hotspot -- what the species
-- detail panel charts (ADR 0004). Missing weeks mean "never recorded", not
-- zero; the caller decides how to render that gap.
SELECT week_of_year, frequency, sample_size
FROM species_seasonality
WHERE hotspot_id = sqlc.arg(hotspot_id) AND species_id = sqlc.arg(species_id)
ORDER BY week_of_year;

-- name: TruncateSpeciesSeasonality :exec
TRUNCATE species_seasonality;

-- name: RecomputeSpeciesSeasonality :execrows
-- Rebuilds the whole table from checklists and observations: per hotspot x
-- species x ISO week, the checklists that recorded the species divided by all
-- complete checklists at the hotspot that week -- the definition eBird's own
-- bar charts use. Only complete checklists (all_species_reported) count,
-- because only they say a missing species was not seen. A shared checklist
-- counts once, whoever submitted a copy of it. sample_size is that count of
-- complete checklists. hotspot_id IS NOT NULL excludes personal and incidental
-- locations -- a per-hotspot stat needs a hotspot.
WITH complete_checklists AS (
    SELECT id, hotspot_id, EXTRACT(week FROM observed_on)::smallint AS week_of_year,
           COALESCE(group_id, sampling_event_id) AS checklist_key
    FROM checklists
    WHERE hotspot_id IS NOT NULL AND all_species_reported
),
week_totals AS (
    SELECT hotspot_id, week_of_year, COUNT(DISTINCT checklist_key) AS checklists
    FROM complete_checklists
    GROUP BY hotspot_id, week_of_year
),
species_totals AS (
    SELECT c.hotspot_id, c.week_of_year, o.species_id, COUNT(DISTINCT c.checklist_key) AS checklists
    FROM complete_checklists c
    JOIN observations o ON o.checklist_id = c.id
    GROUP BY c.hotspot_id, c.week_of_year, o.species_id
)
INSERT INTO species_seasonality (hotspot_id, species_id, week_of_year, frequency, sample_size)
SELECT s.hotspot_id, s.species_id, s.week_of_year,
       s.checklists::real / w.checklists::real,
       w.checklists
FROM species_totals s
JOIN week_totals w USING (hotspot_id, week_of_year);
