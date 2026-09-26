package birding

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/Kontora20/where-to-bird-today/internal/routing"
	"github.com/Kontora20/where-to-bird-today/internal/store"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// recordingDriveTimes answers every call and remembers the origin it was
// asked from, in the order asked.
type recordingDriveTimes struct {
	origins *[]routing.Point
}

func (r recordingDriveTimes) DriveTimes(_ context.Context, origin routing.Point, destinations []routing.Point) ([]time.Duration, error) {
	*r.origins = append(*r.origins, origin)
	return make([]time.Duration, len(destinations)), nil
}

func TestWarmDriveTimeCacheAsksForEveryHotspot(t *testing.T) {
	t.Parallel()

	hotspots := fakeAreaStore{hotspotsWithin: []store.ListHotspotsWithinRow{hotspotWithin(1), hotspotWithin(2)}}
	var gotDestinations []routing.Point
	driveTimes := fakeDriveTimes{gotDestinations: &gotDestinations}
	origin := routing.Point{Latitude: 54.6872, Longitude: 25.2797}

	WarmDriveTimeCache(context.Background(), discardLogger(), hotspots, driveTimes, []routing.Point{origin})

	if len(gotDestinations) != len(hotspots.hotspotsWithin) {
		t.Fatalf("asked for %d destinations, want %d", len(gotDestinations), len(hotspots.hotspotsWithin))
	}
}

func TestWarmDriveTimeCacheWarmsEveryOrigin(t *testing.T) {
	t.Parallel()

	hotspots := fakeAreaStore{hotspotsWithin: []store.ListHotspotsWithinRow{hotspotWithin(1)}}
	var gotOrigins []routing.Point
	driveTimes := recordingDriveTimes{origins: &gotOrigins}
	origins := []routing.Point{
		{Latitude: 54.6872, Longitude: 25.2797},
		{Latitude: 54.8985, Longitude: 23.9036},
		{Latitude: 55.7033, Longitude: 21.1443},
	}

	WarmDriveTimeCache(context.Background(), discardLogger(), hotspots, driveTimes, origins)

	if !slices.Equal(gotOrigins, origins) {
		t.Errorf("asked from origins %v, want %v", gotOrigins, origins)
	}
}

func TestWarmDriveTimeCacheStopsOnAListingFailure(t *testing.T) {
	t.Parallel()

	var gotDestinations []routing.Point
	driveTimes := fakeDriveTimes{gotDestinations: &gotDestinations}
	hotspots := fakeAreaStore{hotspotsWithinErr: errors.New("db down")}

	WarmDriveTimeCache(context.Background(), discardLogger(), hotspots, driveTimes, []routing.Point{{}})

	if gotDestinations != nil {
		t.Error("DriveTimes was called after ListHotspotsWithin failed")
	}
}

func TestWarmDriveTimeCacheDoesNotPanicOnARoutingFailure(t *testing.T) {
	t.Parallel()

	hotspots := fakeAreaStore{hotspotsWithin: []store.ListHotspotsWithinRow{hotspotWithin(1)}}
	driveTimes := fakeDriveTimes{err: errors.New("osrm unreachable")}

	WarmDriveTimeCache(context.Background(), discardLogger(), hotspots, driveTimes, []routing.Point{{}})
}
