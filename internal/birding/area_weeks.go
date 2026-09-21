package birding

import (
	"context"
	"fmt"
	"time"

	"github.com/Kontora20/where-to-bird-today/internal/routing"
	"github.com/Kontora20/where-to-bird-today/internal/store"
)

// AreaParams is one request for the best weeks of some species in an area. The
// area is given as in Params: a radius, or a drive time that replaces it.
// SpeciesIDs must not be empty. Only the calendar date of Date is used.
type AreaParams struct {
	Latitude   float64
	Longitude  float64
	RadiusKM   float64
	MaxMinutes float64
	SpeciesIDs []int64
	Date       time.Time
}

// AreaWeeks is the year for some species over the hotspots of an area. Week is
// the week of the requested date. Each week's Frequency is, for several
// species, the number of them a checklist is expected to have. Weeks and
// BestWeeks are empty lists, never nil, when the area has no checklists.
type AreaWeeks struct {
	Week      int16           `json:"week"`
	Weeks     []WeekFrequency `json:"weeks"`
	BestWeeks []WeekRange     `json:"best_weeks"`
}

// AreaWeeks pools the weekly checklists of every hotspot in the area and finds
// the weeks in which the species are most likely to be seen. An area has so
// many checklists in a week that, unlike at one hotspot, the pooled share needs
// no adjustment toward the country's.
func (s *Service) AreaWeeks(ctx context.Context, p AreaParams) (AreaWeeks, error) {
	radiusKM, err := s.searchRadiusKM(p.RadiusKM, p.MaxMinutes)
	if err != nil {
		return AreaWeeks{}, err
	}

	hotspots, err := s.store.ListHotspotsWithin(ctx, store.ListHotspotsWithinParams{
		Latitude:     p.Latitude,
		Longitude:    p.Longitude,
		RadiusMeters: radiusKM * 1000,
	})
	if err != nil {
		return AreaWeeks{}, fmt.Errorf("finding hotspots: %w", err)
	}

	if p.MaxMinutes > 0 {
		destinations := make([]routing.Point, len(hotspots))
		for i, hotspot := range hotspots {
			destinations[i] = routing.Point{Latitude: hotspot.Latitude.Float64, Longitude: hotspot.Longitude.Float64}
		}
		positions, _, err := s.reachable(ctx, routing.Point{Latitude: p.Latitude, Longitude: p.Longitude}, destinations, p.MaxMinutes)
		if err != nil {
			return AreaWeeks{}, err
		}
		reachable := make([]store.ListHotspotsWithinRow, len(positions))
		for i, position := range positions {
			reachable[i] = hotspots[position]
		}
		hotspots = reachable
	}

	result := AreaWeeks{Week: WeekOfYear(p.Date), Weeks: []WeekFrequency{}, BestWeeks: []WeekRange{}}
	if len(hotspots) == 0 {
		return result, nil
	}

	hotspotIDs := make([]int64, len(hotspots))
	for i, hotspot := range hotspots {
		hotspotIDs[i] = hotspot.ID
	}
	rows, err := s.store.GetAreaWeeks(ctx, store.GetAreaWeeksParams{HotspotIds: hotspotIDs, SpeciesIds: p.SpeciesIDs})
	if err != nil {
		return AreaWeeks{}, fmt.Errorf("loading area weeks: %w", err)
	}

	counts := make([]weekCounts, len(rows))
	for i, row := range rows {
		counts[i] = weekCounts{week: row.WeekOfYear, checklists: row.Checklists, withSpecies: row.WithSpecies}
	}
	year, _ := p.Date.ISOWeek()
	result.Weeks, result.BestWeeks = poolWeeks(counts, year, 0)
	return result, nil
}
