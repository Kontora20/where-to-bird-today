-- name: ListChecklistsForSunrise :many
-- The complete checklists at hotspots that have a start time, with where they
-- were made: what is needed to work out how long after sunrise each started.
SELECT c.id, c.observed_on, c.started_at,
       ST_Y(h.location::geometry) AS latitude,
       ST_X(h.location::geometry) AS longitude
FROM checklists c
JOIN hotspots h ON h.id = c.hotspot_id
WHERE c.all_species_reported AND c.started_at IS NOT NULL;

-- name: ClearChecklistMinutesAfterSunrise :exec
UPDATE checklists SET minutes_after_sunrise = NULL WHERE minutes_after_sunrise IS NOT NULL;

-- name: SetChecklistMinutesAfterSunrise :exec
-- Sets many checklists in one round trip: ids and minutes line up by position.
UPDATE checklists c
SET minutes_after_sunrise = u.minutes
FROM unnest(sqlc.arg(ids)::bigint[], sqlc.arg(minutes)::smallint[]) AS u(id, minutes)
WHERE c.id = u.id;

-- name: TruncateTimeOfDay :exec
TRUNCATE time_of_day_totals, species_time_of_day;

-- name: RecomputeTimeOfDayTotals :execrows
-- Complete checklists at each hotspot per hour after sunrise. A shared
-- checklist counts once, whoever submitted a copy of it.
INSERT INTO time_of_day_totals (hotspot_id, hour_bin, checklists)
SELECT hotspot_id, (minutes_after_sunrise / 60)::smallint, COUNT(DISTINCT COALESCE(group_id, sampling_event_id))
FROM checklists
WHERE hotspot_id IS NOT NULL AND all_species_reported AND minutes_after_sunrise IS NOT NULL
GROUP BY hotspot_id, minutes_after_sunrise / 60;

-- name: RecomputeSpeciesTimeOfDay :execrows
-- Of those, the checklists that recorded each species.
INSERT INTO species_time_of_day (hotspot_id, species_id, hour_bin, checklists)
SELECT c.hotspot_id, o.species_id, (c.minutes_after_sunrise / 60)::smallint, COUNT(DISTINCT COALESCE(c.group_id, c.sampling_event_id))
FROM checklists c
JOIN observations o ON o.checklist_id = c.id
WHERE c.hotspot_id IS NOT NULL AND c.all_species_reported AND c.minutes_after_sunrise IS NOT NULL
GROUP BY c.hotspot_id, o.species_id, c.minutes_after_sunrise / 60;

-- name: GetSpeciesTimeOfDay :many
-- How likely a checklist started in each hour after sunrise is to record the
-- species at the hotspot: the checklists that did, out of those started then,
-- and adjusted_frequency, which pulls that share toward the region's share
-- for the same species and hour. The region is every hotspot loaded, the three
-- Baltic countries together: at night a single country has only a handful of
-- checklists per hour. The region's share counts as prior_checklists extra
-- checklists at the hotspot, the same adjustment as
-- species_seasonality.adjusted_frequency, so an hour where the hotspot has no
-- checklists gets the region's share alone.
--
-- A quiet hour has few checklists even across the region, and one sighting
-- there would look like a peak. So the region's share is itself pulled toward
-- the species' share over all hours, by region_prior_checklists. An hour the
-- region has fewer than min_region_checklists checklists in is left out.
WITH region_totals AS (
    SELECT hour_bin, SUM(checklists) AS checklists
    FROM time_of_day_totals
    GROUP BY hour_bin
),
region_species AS (
    SELECT hour_bin, SUM(checklists) AS checklists
    FROM species_time_of_day
    WHERE species_id = sqlc.arg(species_id)
    GROUP BY hour_bin
),
all_hours AS (
    SELECT COALESCE((SELECT SUM(checklists) FROM region_species)::real / NULLIF((SELECT SUM(checklists) FROM region_totals)::real, 0), 0) AS share
),
region_shares AS (
    SELECT rt.hour_bin, rt.checklists,
           (COALESCE(rs.checklists, 0) + sqlc.arg(region_prior_checklists)::real * ah.share)
               / (rt.checklists + sqlc.arg(region_prior_checklists)::real) AS share
    FROM region_totals rt
    LEFT JOIN region_species rs ON rs.hour_bin = rt.hour_bin
    CROSS JOIN all_hours ah
)
SELECT
    r.hour_bin,
    COALESCE(t.checklists, 0)::integer AS checklists,
    COALESCE(s.checklists, 0)::integer AS with_species,
    ((COALESCE(s.checklists, 0) + sqlc.arg(prior_checklists)::real * r.share)
        / (COALESCE(t.checklists, 0) + sqlc.arg(prior_checklists)::real))::real AS adjusted_frequency
FROM region_shares r
LEFT JOIN time_of_day_totals t ON t.hotspot_id = sqlc.arg(hotspot_id) AND t.hour_bin = r.hour_bin
LEFT JOIN species_time_of_day s ON s.hotspot_id = sqlc.arg(hotspot_id) AND s.species_id = sqlc.arg(species_id) AND s.hour_bin = r.hour_bin
WHERE r.checklists >= sqlc.arg(min_region_checklists)
ORDER BY r.hour_bin;
