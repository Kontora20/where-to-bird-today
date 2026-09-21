package birding

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Kontora20/where-to-bird-today/internal/store"
)

var vilnius = &store.GetHotspotRow{
	ID:        1,
	Latitude:  pgtypeFloat8(54.6872),
	Longitude: pgtypeFloat8(25.2797),
}

// Vilnius sunrise on 2026-06-21 is 04:41:34 local time, shown as 04:42.
func TestTimeOfDayShowsClockTimesForTheDate(t *testing.T) {
	t.Parallel()

	var gotParams store.GetSpeciesTimeOfDayParams
	fake := fakeTimeOfDayStore{
		hotspotRow: vilnius,
		timeOfDayRows: []store.GetSpeciesTimeOfDayRow{
			{HourBin: 0, Checklists: 4, WithSpecies: 2, AdjustedFrequency: 0.44},
			{HourBin: 23, Checklists: 0, WithSpecies: 0, AdjustedFrequency: 0.12},
		},
		timeOfDayParams: &gotParams,
	}

	got, err := timeOfDay(context.Background(), fake, 1, 10, time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("timeOfDay() error = %v", err)
	}

	want := []HourFrequency{
		{HoursAfterSunrise: 0, Start: "04:42", End: "05:42", Frequency: 0.5, Checklists: 4, AdjustedFrequency: 0.44},
		{HoursAfterSunrise: 23, Start: "03:42", End: "04:42", Frequency: 0, Checklists: 0, AdjustedFrequency: 0.12},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("timeOfDay() = %+v, want %+v", got, want)
	}

	if gotParams.HotspotID.Int64 != 1 || gotParams.SpeciesID.Int64 != 10 ||
		gotParams.PriorChecklists.Float32 != timeOfDayPriorChecklists ||
		gotParams.RegionPriorChecklists.Float32 != regionPriorChecklists ||
		gotParams.MinRegionChecklists.Int64 != minRegionChecklistsPerHour {
		t.Errorf("GetSpeciesTimeOfDay params = %+v, want hotspot 1, species 10 and the package constants", gotParams)
	}
}

// The same hour after sunrise is a different clock time in winter.
func TestTimeOfDayFollowsTheSeason(t *testing.T) {
	t.Parallel()

	fake := fakeTimeOfDayStore{
		hotspotRow:    vilnius,
		timeOfDayRows: []store.GetSpeciesTimeOfDayRow{{HourBin: 0, Checklists: 1, WithSpecies: 1, AdjustedFrequency: 0.3}},
	}

	got, err := timeOfDay(context.Background(), fake, 1, 10, time.Date(2026, 12, 21, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("timeOfDay() error = %v", err)
	}

	// Vilnius sunrise on 2026-12-21 is 08:39:53 local time.
	if len(got) != 1 || got[0].Start != "08:40" {
		t.Errorf("timeOfDay() = %+v, want one hour starting at 08:40", got)
	}
}

func TestTimeOfDayIsEmptyForAnUnknownHotspot(t *testing.T) {
	t.Parallel()

	got, err := timeOfDay(context.Background(), fakeTimeOfDayStore{}, 1, 10, time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("timeOfDay() error = %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("timeOfDay() = %#v, want an empty, non-nil list", got)
	}
}

func TestTimeOfDayPropagatesStoreError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("db down")
	fake := fakeTimeOfDayStore{hotspotRow: vilnius, timeOfDayErr: wantErr}

	_, err := timeOfDay(context.Background(), fake, 1, 10, time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC))
	if !errors.Is(err, wantErr) {
		t.Errorf("timeOfDay() error = %v, want wrapping %v", err, wantErr)
	}
}
