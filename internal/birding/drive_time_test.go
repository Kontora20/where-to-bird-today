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

type fakeDriveTimes struct {
	times []time.Duration
	err   error
	// gotDestinations is a pointer because the method has a value receiver.
	gotDestinations *[]routing.Point
}

func (f fakeDriveTimes) DriveTimes(_ context.Context, _ routing.Point, destinations []routing.Point) ([]time.Duration, error) {
	if f.gotDestinations != nil {
		*f.gotDestinations = destinations
	}
	return f.times, f.err
}

func hotspotRow(id int64, latitude float64) store.RecommendHotspotsRow {
	return store.RecommendHotspotsRow{
		HotspotID: id, HotspotName: "Hotspot", Latitude: pgtypeFloat8(latitude), Longitude: pgtypeFloat8(25),
		SpeciesID: pgtypeInt8(10 + id), ScientificName: pgtypeText("Testus"), Frequency: pgtypeFloat4(0.5),
	}
}

func TestRecommendLimitsByDriveTime(t *testing.T) {
	t.Parallel()

	var gotArgs store.RecommendHotspotsParams
	var gotDestinations []routing.Point
	fake := fakeStore{
		recommendArgs: &gotArgs,
		rows:          []store.RecommendHotspotsRow{hotspotRow(1, 54.1), hotspotRow(2, 54.2), hotspotRow(3, 54.3), hotspotRow(4, 54.4)},
	}
	driveTimes := fakeDriveTimes{
		gotDestinations: &gotDestinations,
		times: []time.Duration{
			20*time.Minute + time.Second, // rounds up to 21
			routing.Unreachable,
			31 * time.Minute, // over the limit
			10 * time.Second, // rounds up to 1, never 0
		},
	}

	got, err := NewService(fake, driveTimes).Recommend(context.Background(), Params{Latitude: 54, Longitude: 25, MaxMinutes: 30, Week: 20, Lang: "en"})
	if err != nil {
		t.Fatalf("Recommend() error = %v", err)
	}

	var gotIDs, gotMinutes []int64
	for _, recommendation := range got {
		gotIDs = append(gotIDs, recommendation.HotspotID)
		gotMinutes = append(gotMinutes, int64(recommendation.DriveMinutes))
	}
	if !slices.Equal(gotIDs, []int64{1, 4}) || !slices.Equal(gotMinutes, []int64{21, 1}) {
		t.Errorf("got hotspots %v with minutes %v, want [1 4] with [21 1]", gotIDs, gotMinutes)
	}

	if want := 30 * 130.0 / 60 * 1000; math.Abs(gotArgs.RadiusMeters.Float64-want) > 1e-6 {
		t.Errorf("ranking radius = %v m, want %v m (30 minutes at 130 km/h)", gotArgs.RadiusMeters.Float64, want)
	}
	if len(gotDestinations) != 4 || gotDestinations[2].Latitude != 54.3 {
		t.Errorf("destinations = %+v, want the four hotspots in ranking order", gotDestinations)
	}
}

func TestRecommendWithNoHotspotWithinDriveTimeReturnsEmptySliceNotNil(t *testing.T) {
	t.Parallel()

	fake := fakeStore{rows: []store.RecommendHotspotsRow{hotspotRow(1, 54.1)}}
	driveTimes := fakeDriveTimes{times: []time.Duration{time.Hour}}

	got, err := NewService(fake, driveTimes).Recommend(context.Background(), Params{MaxMinutes: 30, Lang: "en"})
	if err != nil {
		t.Fatalf("Recommend() error = %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("Recommend() = %#v, want a non-nil empty slice", got)
	}
}

func TestRecommendByRadiusIgnoresDriveTimes(t *testing.T) {
	t.Parallel()

	fake := fakeStore{rows: []store.RecommendHotspotsRow{hotspotRow(1, 54.1)}}

	got, err := NewService(fake, fakeDriveTimes{err: errors.New("must not be called")}).Recommend(context.Background(), Params{RadiusKM: 25, Lang: "en"})
	if err != nil {
		t.Fatalf("Recommend() error = %v", err)
	}
	if len(got) != 1 || got[0].DriveMinutes != 0 {
		t.Errorf("Recommend() = %+v, want one hotspot with no drive time", got)
	}
}

func TestRecommendByDriveTimeWithoutRoutingServer(t *testing.T) {
	t.Parallel()

	_, err := NewService(fakeStore{recommendErr: errors.New("must not be called")}, nil).Recommend(context.Background(), Params{MaxMinutes: 30})
	if !errors.Is(err, ErrTravelTimeUnavailable) {
		t.Errorf("Recommend() error = %v, want %v", err, ErrTravelTimeUnavailable)
	}
}

func TestRecommendReportsOriginOffRoad(t *testing.T) {
	t.Parallel()

	fake := fakeStore{rows: []store.RecommendHotspotsRow{hotspotRow(1, 54.1)}}

	_, err := NewService(fake, fakeDriveTimes{err: routing.ErrOriginOffRoad}).Recommend(context.Background(), Params{MaxMinutes: 30})
	if !errors.Is(err, ErrNoRoadNearOrigin) {
		t.Errorf("Recommend() error = %v, want %v", err, ErrNoRoadNearOrigin)
	}
}
