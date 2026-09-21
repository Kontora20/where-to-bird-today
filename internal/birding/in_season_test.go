package birding

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Kontora20/where-to-bird-today/internal/store"
)

// areaWithChecklists is an area whose hotspots have 20 checklists in every week
// between them.
func areaWithChecklists() fakeAreaStore {
	rows := make([]store.GetAreaWeeksRow, weeksPerYear)
	for i := range rows {
		rows[i] = store.GetAreaWeeksRow{WeekOfYear: int16(i + 1), Checklists: 20}
	}
	return fakeAreaStore{
		hotspotsWithin: []store.ListHotspotsWithinRow{{ID: 1}, {ID: 2}},
		areaWeeksRows:  rows,
	}
}

// speciesInWeeks is the rows of a species that was recorded on the given number
// of checklists in each week from firstWeek on.
func speciesInWeeks(speciesID int64, scientificName string, firstWeek int16, counts ...int32) []store.GetAreaSpeciesWeeksRow {
	rows := make([]store.GetAreaSpeciesWeeksRow, len(counts))
	for i, count := range counts {
		rows[i] = store.GetAreaSpeciesWeeksRow{SpeciesID: speciesID, ScientificName: scientificName, WeekOfYear: firstWeek + int16(i), WithSpecies: count}
	}
	return rows
}

func TestInSeasonPicksTheSpeciesAtTheirBest(t *testing.T) {
	t.Parallel()

	area := areaWithChecklists()
	area.nameCandidates = map[int64]map[string]string{10: {"en": "White Stork", "lt": "Baltasis gandras"}}
	var rows []store.GetAreaSpeciesWeeksRow
	rows = append(rows, speciesInWeeks(10, "Ciconia ciconia", 38, 14, 14, 14, 14)...) // still to come: peaks in week 39
	rows = append(rows, speciesInWeeks(11, "Parus major", 1, repeat(52, 10)...)...)   // the same all year: no lift
	rows = append(rows, speciesInWeeks(12, "Rare bird", 39, 1)...)                    // too few checklists
	rows = append(rows, speciesInWeeks(13, "Grus grus", 35, 14, 12, 10, 8, 6)...)     // peaked in week 36
	rows = append(rows, speciesInWeeks(14, "Distant", 9, 18, 18, 18)...)              // peaked in week 10, but common enough now
	rows = append(rows, speciesInWeeks(14, "Distant", 37, 8, 8, 8)...)
	area.speciesWeeksRows = rows

	// 16 September 2026 is in ISO week 38.
	got, err := newWeekFinder(area, nil).InSeason(context.Background(), AreaParams{RadiusKM: 30, Date: time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)}, "lt")
	if err != nil {
		t.Fatalf("InSeason() error = %v", err)
	}

	if got.Week != 38 || len(got.Checklists) != weeksPerYear || got.Checklists[0].Start != "2025-12-29" || got.Checklists[37].Checklists != 20 {
		t.Errorf("Week = %d, Checklists = %d weeks (first %+v), want week 38 and 52 weeks from 2025-12-29", got.Week, len(got.Checklists), got.Checklists[0])
	}

	type summary struct {
		id          int64
		name        string
		weeksToPeak int
		peakStart   string
	}
	var gotSummary []summary
	for _, species := range got.Species {
		gotSummary = append(gotSummary, summary{species.SpeciesID, species.CommonName, species.WeeksToPeak, species.PeakStart})
		if len(species.Year) != weeksPerYear {
			t.Errorf("species %d has %d weeks in Year, want %d", species.SpeciesID, len(species.Year), weeksPerYear)
		}
	}
	want := []summary{
		{10, "Baltasis gandras", 1, "2026-09-21"}, // the most likely first
		{13, "Grus grus", -2, "2026-08-31"},       // no name: the scientific name stands in
	}
	if !slices.Equal(gotSummary, want) {
		t.Errorf("species = %+v, want %+v", gotSummary, want)
	}
}

func TestInSeasonFindsAPeakAcrossTheTurnOfTheYear(t *testing.T) {
	t.Parallel()

	area := areaWithChecklists()
	area.speciesWeeksRows = speciesInWeeks(20, "Bombycilla garrulus", 1, 14, 14, 14)

	// 23 December 2026 is in ISO week 52.
	got, err := newWeekFinder(area, nil).InSeason(context.Background(), AreaParams{RadiusKM: 30, Date: time.Date(2026, 12, 23, 0, 0, 0, 0, time.UTC)}, "en")
	if err != nil {
		t.Fatalf("InSeason() error = %v", err)
	}

	if len(got.Species) != 1 {
		t.Fatalf("species = %+v, want one", got.Species)
	}
	if species := got.Species[0]; species.WeeksToPeak != 2 || species.PeakWeek != 2 || species.PeakStart != "2027-01-11" {
		t.Errorf("WeeksToPeak = %d, PeakWeek = %d, PeakStart = %s, want 2, 2 and 2027-01-11", species.WeeksToPeak, species.PeakWeek, species.PeakStart)
	}
}

func TestInSeasonReturnsOnlyTheBestFew(t *testing.T) {
	t.Parallel()

	area := areaWithChecklists()
	for id := int64(30); id < 38; id++ {
		area.speciesWeeksRows = append(area.speciesWeeksRows, speciesInWeeks(id, "Species", 38, 14, 14, 14, 14)...)
	}

	got, err := newWeekFinder(area, nil).InSeason(context.Background(), AreaParams{RadiusKM: 30, Date: time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)}, "en")
	if err != nil {
		t.Fatalf("InSeason() error = %v", err)
	}

	var ids []int64
	for _, species := range got.Species {
		ids = append(ids, species.SpeciesID)
	}
	if want := []int64{30, 31, 32, 33, 34, 35}; !slices.Equal(ids, want) {
		t.Errorf("species ids = %v, want %v (six, equal ones in id order)", ids, want)
	}
}

func TestInSeasonWithNothingToShow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		store fakeAreaStore
	}{
		{name: "no hotspots in the area", store: fakeAreaStore{}},
		{name: "hotspots without checklists", store: fakeAreaStore{hotspotsWithin: []store.ListHotspotsWithinRow{{ID: 1}}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := newWeekFinder(tc.store, nil).InSeason(context.Background(), AreaParams{RadiusKM: 30, Date: time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)}, "en")
			if err != nil {
				t.Fatalf("InSeason() error = %v", err)
			}
			if got.Checklists == nil || got.Species == nil || len(got.Checklists) != 0 || len(got.Species) != 0 {
				t.Errorf("InSeason() = %+v, want empty lists that are not nil", got)
			}
		})
	}
}

func TestInSeasonReportsAStoreFailure(t *testing.T) {
	t.Parallel()

	area := areaWithChecklists()
	area.speciesWeeksErr = errors.New("boom")

	_, err := newWeekFinder(area, nil).InSeason(context.Background(), AreaParams{RadiusKM: 30, Date: time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)}, "en")
	if !errors.Is(err, area.speciesWeeksErr) {
		t.Errorf("InSeason() error = %v, want it to wrap %v", err, area.speciesWeeksErr)
	}
}

func repeat(n int, value int32) []int32 {
	values := make([]int32, n)
	for i := range values {
		values[i] = value
	}
	return values
}
