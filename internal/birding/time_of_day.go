package birding

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Kontora20/where-to-bird-today/internal/daylight"
	"github.com/Kontora20/where-to-bird-today/internal/store"
)

// timeOfDayPriorChecklists is how many checklists the region's share counts
// as when an hour's share at a hotspot is adjusted. It is the same adjustment
// as species_seasonality.adjusted_frequency, kept as its own constant because
// an hour holds fewer checklists than a week does.
const timeOfDayPriorChecklists = 5

// regionPriorChecklists is how many checklists the species' share over all
// hours counts as when the region's share for one hour is adjusted, so that a
// quiet hour with a single sighting does not look like a peak. Twenty is
// larger than the 12 checklists of the quietest hour, so that hour is mostly
// the all-hours share, and small next to the 937 of the hour before sunrise.
const regionPriorChecklists = 20

// minRegionChecklistsPerHour leaves out an hour after sunrise in which fewer
// checklists than this were started across all three countries: the deepest
// hours of the night, whose share says too little. On the data loaded, the
// quietest hour holds 12, and the next 15, 31 and 42.
const minRegionChecklistsPerHour = 10

// HourFrequency is one hour after sunrise: how likely a checklist started
// then is to record the species. Frequency is the share of the hotspot's
// Checklists that did, as eBird counts it. AdjustedFrequency pulls it toward
// the region's share for the hour, more strongly the fewer Checklists there
// are, and is what the best hours are picked by. An hour with no Checklists
// at the hotspot is the region's share alone.
type HourFrequency struct {
	// HoursAfterSunrise is 0 to 23, wrapping: hour 23 is the hour before
	// sunrise, and the night is the hours in between.
	HoursAfterSunrise int16 `json:"hours_after_sunrise"`
	// Start and End are the clock times of the hour on the requested date,
	// as HH:MM in the hotspot's time zone. End can be earlier than Start
	// only in the sense of passing midnight.
	Start             string  `json:"start"`
	End               string  `json:"end"`
	Frequency         float32 `json:"frequency"`
	Checklists        int32   `json:"checklists"`
	AdjustedFrequency float32 `json:"adjusted_frequency"`
}

// timeOfDay returns the species' hours after sunrise at the hotspot, with the
// clock times they fall at on date. It returns an empty list, not an error,
// when the hotspot is unknown. The list is never nil, which would marshal to
// JSON null.
func (s *Service) timeOfDay(ctx context.Context, hotspotID, speciesID int64, date time.Time) ([]HourFrequency, error) {
	hotspot, err := s.store.GetHotspot(ctx, hotspotID)
	if errors.Is(err, pgx.ErrNoRows) {
		return []HourFrequency{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("loading hotspot: %w", err)
	}
	zone, err := daylight.Zone()
	if err != nil {
		return nil, fmt.Errorf("loading time zone: %w", err)
	}
	year, month, day := date.Date()
	sunrise := daylight.Sunrise(time.Date(year, month, day, 0, 0, 0, 0, zone), hotspot.Latitude.Float64, hotspot.Longitude.Float64).Round(time.Minute)

	rows, err := s.store.GetSpeciesTimeOfDay(ctx, store.GetSpeciesTimeOfDayParams{
		HotspotID:             pgtype.Int8{Int64: hotspotID, Valid: true},
		SpeciesID:             pgtype.Int8{Int64: speciesID, Valid: true},
		PriorChecklists:       pgtype.Float4{Float32: timeOfDayPriorChecklists, Valid: true},
		RegionPriorChecklists: pgtype.Float4{Float32: regionPriorChecklists, Valid: true},
		MinRegionChecklists:   pgtype.Int8{Int64: minRegionChecklistsPerHour, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("loading time of day: %w", err)
	}

	hours := make([]HourFrequency, len(rows))
	for i, row := range rows {
		start := sunrise.Add(time.Duration(row.HourBin) * time.Hour)
		var frequency float32
		if row.Checklists > 0 {
			frequency = float32(row.WithSpecies) / float32(row.Checklists)
		}
		hours[i] = HourFrequency{
			HoursAfterSunrise: row.HourBin,
			Start:             start.Format("15:04"),
			End:               start.Add(time.Hour).Format("15:04"),
			Frequency:         frequency,
			Checklists:        row.Checklists,
			AdjustedFrequency: row.AdjustedFrequency,
		}
	}
	return hours, nil
}
