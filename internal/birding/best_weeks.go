package birding

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/Kontora20/where-to-bird-today/internal/store"
)

// weeksPerYear is the length of the year in weeks. ISO years have 52 or 53
// weeks, and week 53 is counted in week 52, so every year is the same circle.
// The seasonality rebuild folds the data the same way.
const weeksPerYear = 52

// weekPriorChecklists is how many checklists the country's frequency counts
// as when a hotspot's pooled weeks are adjusted: the same adjustment as
// species_seasonality.adjusted_frequency, applied to three weeks at once.
const weekPriorChecklists = 5

// bestWeekShare is how close to the highest week a week must be to count as
// one of the best.
const bestWeekShare = 0.9

// WeekOfYear is the week of the year of date, from 1 to weeksPerYear.
func WeekOfYear(date time.Time) int16 {
	_, week := date.ISOWeek()
	return int16(min(week, weeksPerYear))
}

// WeekFrequency is one week at one hotspot. Frequency is the share of the
// week's Checklists that recorded the species, as eBird counts it, and 0 when
// there were none. SmoothedFrequency pools the week with the weeks either
// side and pulls the result toward the country's frequency, more strongly the
// fewer checklists there are. It is what the best weeks are picked by, and
// where the hotspot has no checklists at all it is the country's alone.
type WeekFrequency struct {
	Week              int16   `json:"week"`
	Frequency         float32 `json:"frequency"`
	Checklists        int32   `json:"checklists"`
	SmoothedFrequency float32 `json:"smoothed_frequency"`
	Best              bool    `json:"best"`
}

// WeekRange is a run of neighbouring best weeks. Start and End are the first
// and last day of the run as YYYY-MM-DD, in the ISO year of the requested
// date; a run that goes past the end of the year ends in the next one.
type WeekRange struct {
	FromWeek int16  `json:"from_week"`
	ToWeek   int16  `json:"to_week"`
	Start    string `json:"start"`
	End      string `json:"end"`
}

// seasonality returns the species' weeks at the hotspot and the ranges of
// best weeks among them. Both are empty lists, never nil, when the hotspot has
// no checklists.
func (s *Service) seasonality(ctx context.Context, hotspotID, speciesID int64, date time.Time) ([]WeekFrequency, []WeekRange, error) {
	rows, err := s.store.GetHotspotSpeciesWeeks(ctx, store.GetHotspotSpeciesWeeksParams{HotspotID: hotspotID, SpeciesID: speciesID})
	if err != nil {
		return nil, nil, fmt.Errorf("loading species seasonality: %w", err)
	}
	year, _ := date.ISOWeek()
	weeks, ranges := bestWeeks(rows, year)
	return weeks, ranges, nil
}

// bestWeeks pools each week with its two neighbours, the year being a circle,
// and adjusts the pooled share toward the average country frequency of those
// weeks. The best weeks are those within bestWeekShare of the highest.
func bestWeeks(rows []store.GetHotspotSpeciesWeeksRow, year int) ([]WeekFrequency, []WeekRange) {
	if !slices.ContainsFunc(rows, func(row store.GetHotspotSpeciesWeeksRow) bool { return row.Checklists > 0 }) {
		return []WeekFrequency{}, []WeekRange{}
	}

	weeks := make([]WeekFrequency, len(rows))
	var highest float32
	for i, row := range rows {
		var checklists, withSpecies int32
		var countryFrequency float32
		for _, offset := range []int{-1, 0, 1} {
			neighbour := rows[(i+offset+len(rows))%len(rows)]
			checklists += neighbour.Checklists
			withSpecies += neighbour.WithSpecies
			countryFrequency += neighbour.CountryFrequency / 3
		}
		smoothed := (float32(withSpecies) + weekPriorChecklists*countryFrequency) / (float32(checklists) + weekPriorChecklists)

		var frequency float32
		if row.Checklists > 0 {
			frequency = float32(row.WithSpecies) / float32(row.Checklists)
		}
		weeks[i] = WeekFrequency{Week: row.WeekOfYear, Frequency: frequency, Checklists: row.Checklists, SmoothedFrequency: smoothed}
		highest = max(highest, smoothed)
	}

	best := make([]bool, len(weeks))
	for i := range weeks {
		best[i] = highest > 0 && weeks[i].SmoothedFrequency >= bestWeekShare*highest
		weeks[i].Best = best[i]
	}
	return weeks, weekRanges(best, year)
}

// weekRanges turns the best weeks into runs of neighbouring weeks, the year
// being a circle: a run can go from the last weeks of the year into the first.
func weekRanges(best []bool, year int) []WeekRange {
	ranges := []WeekRange{}
	if len(best) > 0 && !slices.Contains(best, false) {
		return append(ranges, weekRange(0, len(best)-1, year))
	}
	for first := range best {
		if !best[first] || best[(first+len(best)-1)%len(best)] {
			continue
		}
		last := first
		for best[(last+1)%len(best)] {
			last++
		}
		ranges = append(ranges, weekRange(first, last, year))
	}
	return ranges
}

// weekRange is the range from index first to index last, which is past the end
// of the year when the run wraps into the next.
func weekRange(first, last, year int) WeekRange {
	endYear := year
	if last >= weeksPerYear {
		endYear++
	}
	lastWeek := last%weeksPerYear + 1
	return WeekRange{
		FromWeek: int16(first + 1),
		ToWeek:   int16(lastWeek),
		Start:    isoWeekStart(year, first+1).Format("2006-01-02"),
		End:      isoWeekEnd(endYear, lastWeek).Format("2006-01-02"),
	}
}

// isoWeekStart is the Monday of an ISO week: week 1 is the one with 4 January.
func isoWeekStart(year, week int) time.Time {
	january4 := time.Date(year, time.January, 4, 0, 0, 0, 0, time.UTC)
	daysSinceMonday := int(january4.Weekday()+6) % 7
	return january4.AddDate(0, 0, (week-1)*7-daysSinceMonday)
}

// isoWeekEnd is the last day of a week of the year. The last week runs to the
// end of the ISO year, which is 8 days after its Monday in a 53 week year.
func isoWeekEnd(year, week int) time.Time {
	if week == weeksPerYear {
		return isoWeekStart(year+1, 1).AddDate(0, 0, -1)
	}
	return isoWeekStart(year, week).AddDate(0, 0, 6)
}
