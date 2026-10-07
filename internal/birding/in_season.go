package birding

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/Kontora20/where-to-bird-today/internal/store"
)

const (
	// maxSeasonalSpecies caps how many species InSeason returns.
	maxSeasonalSpecies = 6

	// minSeasonalFrequency is the share of the area's checklists that must
	// have recorded a species in the week for it to be one to look for.
	minSeasonalFrequency = 0.05

	// seasonalLift is how many times its average over the year a species'
	// frequency must be in the week. It leaves out the birds that are
	// everywhere all year.
	seasonalLift = 1.5

	// seasonalWindow is how many weeks from the requested week a species' best
	// week may be, before or after. It leaves out the birds whose season is
	// long past or still far off.
	seasonalWindow = 4

	// yearLength is the number of weeks, as the weekly figures are indexed.
	yearLength = weeksPerYear
)

// WeekTotal is the complete checklists of one week over the hotspots of an
// area. Start is the Monday of the week, as YYYY-MM-DD, in the ISO year of the
// requested date.
type WeekTotal struct {
	Week       int16  `json:"week"`
	Start      string `json:"start"`
	Checklists int32  `json:"checklists"`
}

// SeasonalSpecies is a species that is much more likely in the requested week
// than it usually is, in the area. Frequency is the share of the area's
// checklists that week, pooled with the weeks either side, that recorded it.
// Year is the same figure for each of the weeks of the year. WeeksToPeak is
// how far the week with the highest figure is from the requested week: 0 is
// now, above 0 is still to come and below 0 has passed, by the shorter way
// round the year.
type SeasonalSpecies struct {
	SpeciesID      int64     `json:"species_id"`
	CommonName     string    `json:"common_name"`
	ScientificName string    `json:"scientific_name"`
	Frequency      float32   `json:"frequency"`
	PeakWeek       int16     `json:"peak_week"`
	PeakStart      string    `json:"peak_start"`
	WeeksToPeak    int       `json:"weeks_to_peak"`
	Year           []float32 `json:"year"`
	// Absent when no photo is stored for the species.
	Photo *Photo `json:"photo,omitempty"`
}

// Season is what an area holds in the week of a date: its checklists in each
// week of the year, and the species most worth looking for now, the most
// likely first. Both are empty lists, never nil, when the area has no
// checklists.
type Season struct {
	Week       int16             `json:"week"`
	Checklists []WeekTotal       `json:"checklists"`
	Species    []SeasonalSpecies `json:"species"`
}

// InSeason finds the species that are at their best in the area in the week of
// p.Date: at least minSeasonalFrequency of the checklists record them, that is
// at least seasonalLift times their average over the year, and their best week
// is within seasonalWindow weeks of the requested one. p.SpeciesIDs is not
// used.
func (f *WeekFinder) InSeason(ctx context.Context, p AreaParams, lang string) (Season, error) {
	result := Season{Week: WeekOfYear(p.Date), Checklists: []WeekTotal{}, Species: []SeasonalSpecies{}}

	hotspotIDs, err := f.hotspotsInArea(ctx, p)
	if err != nil {
		return Season{}, err
	}
	if len(hotspotIDs) == 0 {
		return result, nil
	}

	// Without species only the checklists of each week come back.
	totals, err := f.store.GetAreaWeeks(ctx, store.GetAreaWeeksParams{HotspotIds: hotspotIDs})
	if err != nil {
		return Season{}, fmt.Errorf("loading area checklists: %w", err)
	}
	var checklists [yearLength]int32
	var anyChecklists bool
	for _, row := range totals {
		if row.WeekOfYear >= 1 && row.WeekOfYear <= yearLength {
			checklists[row.WeekOfYear-1] = row.Checklists
			anyChecklists = anyChecklists || row.Checklists > 0
		}
	}
	if !anyChecklists {
		return result, nil
	}
	year, _ := p.Date.ISOWeek()
	for i, count := range checklists {
		result.Checklists = append(result.Checklists, WeekTotal{
			Week:       int16(i + 1),
			Start:      isoWeekStart(year, i+1).Format("2006-01-02"),
			Checklists: count,
		})
	}

	rows, err := f.store.GetAreaSpeciesWeeks(ctx, hotspotIDs)
	if err != nil {
		return Season{}, fmt.Errorf("loading species weeks: %w", err)
	}
	type speciesYear struct {
		scientificName string
		withSpecies    [yearLength]int32
	}
	years := make(map[int64]*speciesYear)
	for _, row := range rows {
		if row.WeekOfYear < 1 || row.WeekOfYear > yearLength {
			continue
		}
		known, ok := years[row.SpeciesID]
		if !ok {
			known = &speciesYear{scientificName: row.ScientificName}
			years[row.SpeciesID] = known
		}
		known.withSpecies[row.WeekOfYear-1] = row.WithSpecies
	}

	now := int(result.Week) - 1
	var found []SeasonalSpecies
	for speciesID, known := range years {
		counts := make([]weekCounts, yearLength)
		for i := range counts {
			counts[i] = weekCounts{week: int16(i + 1), checklists: checklists[i], withSpecies: known.withSpecies[i]}
		}
		weeks, _ := poolWeeks(counts, year, 0)

		var sum float32
		peak := 0
		for i, week := range weeks {
			sum += week.SmoothedFrequency
			if week.SmoothedFrequency > weeks[peak].SmoothedFrequency {
				peak = i
			}
		}
		current := weeks[now].SmoothedFrequency
		if current < minSeasonalFrequency || current < seasonalLift*sum/yearLength {
			continue
		}

		toPeak := peak - now
		if toPeak > yearLength/2 {
			toPeak -= yearLength
		} else if toPeak < -yearLength/2 {
			toPeak += yearLength
		}
		if toPeak > seasonalWindow || toPeak < -seasonalWindow {
			continue
		}
		// A peak that lies across the turn of the year is in the next or the
		// previous ISO year.
		peakYear := year
		if now+toPeak >= yearLength {
			peakYear++
		} else if now+toPeak < 0 {
			peakYear--
		}

		smoothed := make([]float32, yearLength)
		for i, week := range weeks {
			smoothed[i] = week.SmoothedFrequency
		}
		found = append(found, SeasonalSpecies{
			SpeciesID:      speciesID,
			ScientificName: known.scientificName,
			Frequency:      current,
			PeakWeek:       int16(peak + 1),
			PeakStart:      isoWeekStart(peakYear, peak+1).Format("2006-01-02"),
			WeeksToPeak:    toPeak,
			Year:           smoothed,
		})
	}

	// The species come out of a map in any order, so ties are broken by id.
	slices.SortFunc(found, func(a, b SeasonalSpecies) int {
		return cmp.Or(cmp.Compare(b.Frequency, a.Frequency), cmp.Compare(a.SpeciesID, b.SpeciesID))
	})
	found = found[:min(len(found), maxSeasonalSpecies)]
	if len(found) == 0 {
		return result, nil
	}

	ids := make([]int64, len(found))
	for i, species := range found {
		ids[i] = species.SpeciesID
	}
	names, err := commonNames(ctx, f.store, ids, lang)
	if err != nil {
		return Season{}, err
	}
	media, err := f.store.ListBestSpeciesMedia(ctx, ids)
	if err != nil {
		return Season{}, fmt.Errorf("loading species photos: %w", err)
	}
	photos := make(map[int64]*Photo, len(media))
	for _, medium := range media {
		photos[medium.SpeciesID] = photoOf(medium)
	}
	for i := range found {
		name, ok := names[found[i].SpeciesID]
		if !ok {
			name = found[i].ScientificName
		}
		found[i].CommonName = name
		found[i].Photo = photos[found[i].SpeciesID]
	}
	result.Species = found
	return result, nil
}
