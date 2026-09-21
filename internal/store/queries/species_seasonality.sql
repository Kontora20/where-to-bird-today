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
-- complete checklists. Joining to hotspots excludes personal and incidental
-- locations -- a per-hotspot stat needs a hotspot.
--
-- adjusted_frequency is (with_species + prior_checklists * country_frequency)
-- / (checklists + prior_checklists): the country's frequency for the same
-- species and week counts as prior_checklists extra checklists at the hotspot.
-- One sighting in 3 checklists moves far from the country figure; 50 in 200
-- barely moves. A species with no sighting at the hotspot that week has no
-- row, so it gets no adjusted frequency either.
WITH complete_checklists AS (
    SELECT c.id, c.hotspot_id, h.region, EXTRACT(week FROM c.observed_on)::smallint AS week_of_year,
           COALESCE(c.group_id, c.sampling_event_id) AS checklist_key
    FROM checklists c
    JOIN hotspots h ON h.id = c.hotspot_id
    WHERE c.all_species_reported
),
week_totals AS (
    SELECT hotspot_id, region, week_of_year, COUNT(DISTINCT checklist_key) AS checklists
    FROM complete_checklists
    GROUP BY hotspot_id, region, week_of_year
),
species_totals AS (
    SELECT c.hotspot_id, c.region, c.week_of_year, o.species_id, COUNT(DISTINCT c.checklist_key) AS checklists
    FROM complete_checklists c
    JOIN observations o ON o.checklist_id = c.id
    GROUP BY c.hotspot_id, c.region, c.week_of_year, o.species_id
),
country_week_totals AS (
    SELECT region, week_of_year, SUM(checklists) AS checklists
    FROM week_totals
    GROUP BY region, week_of_year
),
country_species_totals AS (
    SELECT region, week_of_year, species_id, SUM(checklists) AS checklists
    FROM species_totals
    GROUP BY region, week_of_year, species_id
)
INSERT INTO species_seasonality (hotspot_id, species_id, week_of_year, frequency, adjusted_frequency, sample_size)
SELECT s.hotspot_id, s.species_id, s.week_of_year,
       s.checklists::real / w.checklists::real,
       (s.checklists + sqlc.arg(prior_checklists)::real * cs.checklists::real / cw.checklists::real)
           / (w.checklists + sqlc.arg(prior_checklists)::real),
       w.checklists
FROM species_totals s
JOIN week_totals w USING (hotspot_id, week_of_year)
JOIN country_species_totals cs ON cs.region = s.region AND cs.week_of_year = s.week_of_year AND cs.species_id = s.species_id
JOIN country_week_totals cw ON cw.region = s.region AND cw.week_of_year = s.week_of_year;
