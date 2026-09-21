package birding

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/Kontora20/where-to-bird-today/internal/store"
)

// yearOfWeeks is a full year of rows, each week with the country frequency 0.1
// and no checklists unless it is one of the overrides.
func yearOfWeeks(overrides map[int16]store.GetHotspotSpeciesWeeksRow) []store.GetHotspotSpeciesWeeksRow {
	rows := make([]store.GetHotspotSpeciesWeeksRow, weeksPerYear)
	for i := range rows {
		week := int16(i + 1)
		row, ok := overrides[week]
		if !ok {
			row = store.GetHotspotSpeciesWeeksRow{CountryFrequency: 0.1}
		}
		row.WeekOfYear = week
		rows[i] = row
	}
	return rows
}

func TestWeekOfYearFoldsWeek53IntoWeek52(t *testing.T) {
	t.Parallel()

	tests := []struct {
		date string
		want int16
	}{
		{"2026-05-15", 20},
		{"2026-01-01", 1},
		{"2025-12-29", 1},
		{"2026-12-21", 52},
		{"2026-12-30", 52},
		{"2027-01-02", 52},
	}
	for _, test := range tests {
		date, err := time.Parse("2006-01-02", test.date)
		if err != nil {
			t.Fatalf("parsing %s: %v", test.date, err)
		}
		if got := WeekOfYear(date); got != test.want {
			t.Errorf("WeekOfYear(%s) = %d, want %d", test.date, got, test.want)
		}
	}
}

// Week 20 pools weeks 19 to 21: 5 checklists, 4 with the species, and the
// country's frequencies 0.1, 0.4 and 0.1, which average 0.2. With a prior of
// 5 checklists that is (4 + 5*0.2) / (5 + 5) = 0.5.
func TestBestWeeksPoolsNeighboursAndAdjustsTowardTheCountry(t *testing.T) {
	t.Parallel()

	rows := yearOfWeeks(map[int16]store.GetHotspotSpeciesWeeksRow{
		19: {Checklists: 2, WithSpecies: 1, CountryFrequency: 0.1},
		20: {Checklists: 3, WithSpecies: 3, CountryFrequency: 0.4},
	})

	weeks, _ := bestWeeks(rows, 2026)

	if len(weeks) != weeksPerYear {
		t.Fatalf("got %d weeks, want %d", len(weeks), weeksPerYear)
	}
	week20 := weeks[19]
	if week20.Week != 20 || week20.Frequency != 1 || week20.Checklists != 3 || math.Abs(float64(week20.SmoothedFrequency)-0.5) > 1e-6 {
		t.Errorf("week 20 = %+v, want frequency 1 of 3 checklists, smoothed 0.5", week20)
	}
	// A week with no checklists of its own, far from any, is the country's
	// frequency alone.
	if week40 := weeks[39]; week40.Frequency != 0 || week40.Checklists != 0 || math.Abs(float64(week40.SmoothedFrequency)-0.1) > 1e-6 {
		t.Errorf("week 40 = %+v, want no checklists and smoothed 0.1", week40)
	}
}

// The year is a circle: week 1 pools weeks 52, 1 and 2.
func TestBestWeeksPoolsAcrossTheEndOfTheYear(t *testing.T) {
	t.Parallel()

	rows := yearOfWeeks(map[int16]store.GetHotspotSpeciesWeeksRow{
		52: {Checklists: 4, WithSpecies: 4, CountryFrequency: 0.1},
	})

	weeks, ranges := bestWeeks(rows, 2026)

	// Week 1 pools 4 checklists, 4 with the species, and the country's 0.1:
	// (4 + 5*0.1) / (4 + 5).
	if want := (4 + 5*0.1) / 9.0; math.Abs(float64(weeks[0].SmoothedFrequency)-want) > 1e-6 {
		t.Errorf("week 1 smoothed = %v, want %v", weeks[0].SmoothedFrequency, want)
	}
	// Weeks 51, 52 and 1 all pool the same 4 of 4 and are the best; 51 and 1
	// are neighbours of 52 across the year end.
	want := []WeekRange{{FromWeek: 51, ToWeek: 1, Start: "2026-12-14", End: "2027-01-10"}}
	if !slices.Equal(ranges, want) {
		t.Errorf("ranges = %+v, want %+v", ranges, want)
	}
}

func TestBestWeeksIsEmptyWithoutChecklists(t *testing.T) {
	t.Parallel()

	weeks, ranges := bestWeeks(yearOfWeeks(nil), 2026)

	if weeks == nil || len(weeks) != 0 || ranges == nil || len(ranges) != 0 {
		t.Errorf("bestWeeks() = %#v, %#v, want empty, non-nil lists", weeks, ranges)
	}
}

func TestWeekRanges(t *testing.T) {
	t.Parallel()

	best := func(weeks ...int) []bool {
		flags := make([]bool, weeksPerYear)
		for _, week := range weeks {
			flags[week-1] = true
		}
		return flags
	}
	all := make([]int, weeksPerYear)
	for i := range all {
		all[i] = i + 1
	}

	tests := []struct {
		name string
		best []bool
		want []WeekRange
	}{
		{"none", best(), []WeekRange{}},
		{"one run", best(20, 21, 22), []WeekRange{{FromWeek: 20, ToWeek: 22, Start: "2026-05-11", End: "2026-05-31"}}},
		{"two runs", best(10, 30, 31), []WeekRange{
			{FromWeek: 10, ToWeek: 10, Start: "2026-03-02", End: "2026-03-08"},
			{FromWeek: 30, ToWeek: 31, Start: "2026-07-20", End: "2026-08-02"},
		}},
		{"a run into the next year", best(51, 52, 1, 2), []WeekRange{{FromWeek: 51, ToWeek: 2, Start: "2026-12-14", End: "2027-01-17"}}},
		{"a run to the last week ends with the ISO year", best(50, 51, 52), []WeekRange{{FromWeek: 50, ToWeek: 52, Start: "2026-12-07", End: "2027-01-03"}}},
		{"every week", best(all...), []WeekRange{{FromWeek: 1, ToWeek: 52, Start: "2025-12-29", End: "2027-01-03"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := weekRanges(test.best, 2026); !slices.Equal(got, test.want) {
				t.Errorf("weekRanges() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestIsoWeekStart(t *testing.T) {
	t.Parallel()

	tests := []struct {
		year, week int
		want       string
	}{
		{2026, 1, "2025-12-29"},
		{2027, 1, "2027-01-04"},
		{2025, 1, "2024-12-30"},
		{2026, 20, "2026-05-11"},
	}
	for _, test := range tests {
		if got := isoWeekStart(test.year, test.week).Format("2006-01-02"); got != test.want {
			t.Errorf("isoWeekStart(%d, %d) = %s, want %s", test.year, test.week, got, test.want)
		}
	}
}
