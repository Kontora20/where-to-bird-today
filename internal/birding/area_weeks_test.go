package birding

import (
	"context"
	"errors"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/Kontora20/where-to-bird-today/internal/routing"
	"github.com/Kontora20/where-to-bird-today/internal/store"
)

// yearOfAreaWeeks is a full year of rows with no checklists, except the
// overrides.
func yearOfAreaWeeks(overrides map[int16]store.GetAreaWeeksRow) []store.GetAreaWeeksRow {
	rows := make([]store.GetAreaWeeksRow, weeksPerYear)
	for i := range rows {
		week := int16(i + 1)
		rows[i] = overrides[week]
		rows[i].WeekOfYear = week
	}
	return rows
}

func hotspotWithin(id int64) store.ListHotspotsWithinRow {
	return store.ListHotspotsWithinRow{ID: id, Latitude: pgtypeFloat8(54.5), Longitude: pgtypeFloat8(25.5)}
}

var may15 = time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)

// Weeks 20, 21 and 22 have 10 checklists each, with 5, 6 and 4 of the
// species. Each week pools its neighbours, with no adjustment: week 20 is 11 of
// 20 (0.55), and weeks 19, 21 and 22 are 5 of 10, 15 of 30 and 10 of 20, all
// 0.5, within a tenth of it. Week 23 is 4 of 10 (0.4) and is not.
func TestAreaWeeksPoolsTheHotspotsOfAnArea(t *testing.T) {
	t.Parallel()

	var gotArgs store.GetAreaWeeksParams
	fake := fakeStore{
		hotspotsWithin: []store.ListHotspotsWithinRow{hotspotWithin(1), hotspotWithin(2)},
		areaWeeksRows: yearOfAreaWeeks(map[int16]store.GetAreaWeeksRow{
			20: {Checklists: 10, WithSpecies: 5},
			21: {Checklists: 10, WithSpecies: 6},
			22: {Checklists: 10, WithSpecies: 4},
		}),
		areaWeeksArgs: &gotArgs,
	}

	got, err := NewService(fake, nil).AreaWeeks(context.Background(), AreaParams{
		Latitude: 54.7, Longitude: 25.3, RadiusKM: 30, SpeciesIDs: []int64{10, 11}, Date: may15,
	})
	if err != nil {
		t.Fatalf("AreaWeeks() error = %v", err)
	}

	if !slices.Equal(gotArgs.HotspotIds, []int64{1, 2}) || !slices.Equal(gotArgs.SpeciesIds, []int64{10, 11}) {
		t.Errorf("GetAreaWeeks args = %+v, want hotspots 1 and 2 and species 10 and 11", gotArgs)
	}
	if got.Week != 20 || len(got.Weeks) != weeksPerYear {
		t.Fatalf("Week = %d with %d weeks, want week 20 of %d", got.Week, len(got.Weeks), weeksPerYear)
	}
	if week21 := got.Weeks[20]; week21.Week != 21 || week21.Start != "2026-05-18" || week21.Checklists != 10 || week21.Frequency != 0.6 || week21.SmoothedFrequency != 0.5 {
		t.Errorf("week 21 = %+v, want start 2026-05-18, 6 of 10 checklists, smoothed 0.5", week21)
	}
	if week20 := got.Weeks[19]; math.Abs(float64(week20.SmoothedFrequency)-0.55) > 1e-6 {
		t.Errorf("week 20 smoothed = %v, want 0.55 (11 of 20)", week20.SmoothedFrequency)
	}
	wantBest := []WeekRange{{FromWeek: 19, ToWeek: 22, Start: "2026-05-04", End: "2026-05-31"}}
	if !slices.Equal(got.BestWeeks, wantBest) {
		t.Errorf("BestWeeks = %+v, want %+v", got.BestWeeks, wantBest)
	}
	// A week far from any checklists has no checklists to pool, and is 0, not
	// a division of 0 by 0.
	if week40 := got.Weeks[39]; week40.SmoothedFrequency != 0 || math.IsNaN(float64(week40.SmoothedFrequency)) {
		t.Errorf("week 40 smoothed = %v, want 0", week40.SmoothedFrequency)
	}
}

func TestAreaWeeksLimitsByDriveTime(t *testing.T) {
	t.Parallel()

	var gotArgs store.GetAreaWeeksParams
	var gotDestinations []routing.Point
	fake := fakeStore{
		hotspotsWithin: []store.ListHotspotsWithinRow{hotspotWithin(1), hotspotWithin(2), hotspotWithin(3)},
		areaWeeksRows:  yearOfAreaWeeks(map[int16]store.GetAreaWeeksRow{20: {Checklists: 4, WithSpecies: 2}}),
		areaWeeksArgs:  &gotArgs,
	}
	driveTimes := fakeDriveTimes{
		gotDestinations: &gotDestinations,
		times:           []time.Duration{10 * time.Minute, routing.Unreachable, 40 * time.Minute},
	}

	_, err := NewService(fake, driveTimes).AreaWeeks(context.Background(), AreaParams{
		Latitude: 54.7, Longitude: 25.3, MaxMinutes: 30, SpeciesIDs: []int64{10}, Date: may15,
	})
	if err != nil {
		t.Fatalf("AreaWeeks() error = %v", err)
	}

	if len(gotDestinations) != 3 {
		t.Errorf("drive times asked for %d hotspots, want all 3 within the straight-line distance", len(gotDestinations))
	}
	if !slices.Equal(gotArgs.HotspotIds, []int64{1}) {
		t.Errorf("weeks asked for hotspots %v, want only hotspot 1: 2 is unreachable and 3 is over the limit", gotArgs.HotspotIds)
	}
}

func TestAreaWeeksByRadiusIgnoresDriveTimes(t *testing.T) {
	t.Parallel()

	fake := fakeStore{
		hotspotsWithin: []store.ListHotspotsWithinRow{hotspotWithin(1)},
		areaWeeksRows:  yearOfAreaWeeks(map[int16]store.GetAreaWeeksRow{20: {Checklists: 4, WithSpecies: 2}}),
	}
	driveTimes := fakeDriveTimes{err: errors.New("routing must not be called for a radius")}

	if _, err := NewService(fake, driveTimes).AreaWeeks(context.Background(), AreaParams{RadiusKM: 30, SpeciesIDs: []int64{10}, Date: may15}); err != nil {
		t.Errorf("AreaWeeks() error = %v, want none: a radius needs no drive times", err)
	}
}

func TestAreaWeeksByDriveTimeWithoutRoutingServer(t *testing.T) {
	t.Parallel()

	_, err := NewService(fakeStore{}, nil).AreaWeeks(context.Background(), AreaParams{MaxMinutes: 30, SpeciesIDs: []int64{10}, Date: may15})
	if !errors.Is(err, ErrTravelTimeUnavailable) {
		t.Errorf("AreaWeeks() error = %v, want %v", err, ErrTravelTimeUnavailable)
	}
}

func TestAreaWeeksWithNoHotspotsIsEmpty(t *testing.T) {
	t.Parallel()

	var gotArgs store.GetAreaWeeksParams
	fake := fakeStore{areaWeeksArgs: &gotArgs}

	got, err := NewService(fake, nil).AreaWeeks(context.Background(), AreaParams{RadiusKM: 30, SpeciesIDs: []int64{10}, Date: may15})
	if err != nil {
		t.Fatalf("AreaWeeks() error = %v", err)
	}

	if got.Weeks == nil || len(got.Weeks) != 0 || got.BestWeeks == nil || len(got.BestWeeks) != 0 || got.Week != 20 {
		t.Errorf("AreaWeeks() = %#v, want week 20 with empty, non-nil lists", got)
	}
	if gotArgs.HotspotIds != nil {
		t.Errorf("GetAreaWeeks was asked for hotspots %v, want no query when the area has none", gotArgs.HotspotIds)
	}
}

func TestAreaWeeksWithNoChecklistsIsEmpty(t *testing.T) {
	t.Parallel()

	fake := fakeStore{hotspotsWithin: []store.ListHotspotsWithinRow{hotspotWithin(1)}, areaWeeksRows: yearOfAreaWeeks(nil)}

	got, err := NewService(fake, nil).AreaWeeks(context.Background(), AreaParams{RadiusKM: 30, SpeciesIDs: []int64{10}, Date: may15})
	if err != nil {
		t.Fatalf("AreaWeeks() error = %v", err)
	}
	if got.Weeks == nil || len(got.Weeks) != 0 || got.BestWeeks == nil || len(got.BestWeeks) != 0 {
		t.Errorf("AreaWeeks() = %#v, want empty, non-nil lists", got)
	}
}

func TestAreaWeeksPropagatesStoreError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("db down")
	fake := fakeStore{hotspotsWithin: []store.ListHotspotsWithinRow{hotspotWithin(1)}, areaWeeksErr: wantErr}

	_, err := NewService(fake, nil).AreaWeeks(context.Background(), AreaParams{RadiusKM: 30, SpeciesIDs: []int64{10}, Date: may15})
	if !errors.Is(err, wantErr) {
		t.Errorf("AreaWeeks() error = %v, want wrapping %v", err, wantErr)
	}
}
