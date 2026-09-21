// Package timeofday rebuilds the counts behind the time-of-day profile of a
// species: for each hotspot, how many complete checklists started in each
// hour after sunrise, and how many of them recorded each species. Like
// seasonality it has no external source, only checklists already in Postgres.
package timeofday

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Kontora20/where-to-bird-today/internal/daylight"
	"github.com/Kontora20/where-to-bird-today/internal/store"
)

// Source is the ingestion_runs.source value this job writes.
const Source = "time-of-day"

// Recompute works out how long after sunrise each complete hotspot checklist
// started, then rebuilds the two count tables from that. It returns the number
// of rows written to them.
func Recompute(ctx context.Context, queries *store.Queries) (int, error) {
	checklists, err := queries.ListChecklistsForSunrise(ctx)
	if err != nil {
		return 0, fmt.Errorf("listing checklists: %w", err)
	}

	zone, err := daylight.Zone()
	if err != nil {
		return 0, fmt.Errorf("loading time zone: %w", err)
	}

	ids := make([]int64, 0, len(checklists))
	minutes := make([]int16, 0, len(checklists))
	for _, checklist := range checklists {
		start := localStart(checklist.ObservedOn.Time, checklist.StartedAt, zone)
		ids = append(ids, checklist.ID)
		minutes = append(minutes, int16(daylight.MinutesAfterSunrise(start, checklist.Latitude.Float64, checklist.Longitude.Float64)))
	}

	if err := queries.ClearChecklistMinutesAfterSunrise(ctx); err != nil {
		return 0, fmt.Errorf("clearing minutes after sunrise: %w", err)
	}
	if err := queries.SetChecklistMinutesAfterSunrise(ctx, store.SetChecklistMinutesAfterSunriseParams{Ids: ids, Minutes: minutes}); err != nil {
		return 0, fmt.Errorf("setting minutes after sunrise: %w", err)
	}

	if err := queries.TruncateTimeOfDay(ctx); err != nil {
		return 0, fmt.Errorf("clearing time-of-day counts: %w", err)
	}
	totals, err := queries.RecomputeTimeOfDayTotals(ctx)
	if err != nil {
		return 0, fmt.Errorf("recomputing time_of_day_totals: %w", err)
	}
	species, err := queries.RecomputeSpeciesTimeOfDay(ctx)
	if err != nil {
		return 0, fmt.Errorf("recomputing species_time_of_day: %w", err)
	}
	return int(totals + species), nil
}

// localStart is the moment the clock in zone read startedAt on the day
// observedOn. The clock reading is set field by field rather than added to
// midnight, because on a day the clocks change an added duration lands an hour
// off the reading.
func localStart(observedOn time.Time, startedAt pgtype.Time, zone *time.Location) time.Time {
	sinceMidnight := time.Duration(startedAt.Microseconds) * time.Microsecond
	year, month, day := observedOn.Date()
	return time.Date(year, month, day,
		int(sinceMidnight.Hours()), int(sinceMidnight.Minutes())%60, int(sinceMidnight.Seconds())%60, 0, zone)
}
