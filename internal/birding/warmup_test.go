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

// signalingDriveTimes tells the test each time it is asked, and answers with err.
type signalingDriveTimes struct {
	asked chan struct{}
	err   error
}

func (s signalingDriveTimes) DriveTimes(_ context.Context, _ routing.Point, destinations []routing.Point) ([]time.Duration, error) {
	s.asked <- struct{}{}
	return make([]time.Duration, len(destinations)), s.err
}

func TestRoutingWarmerWakesTheServerOnceForManyRequests(t *testing.T) {
	t.Parallel()

	server := signalingDriveTimes{asked: make(chan struct{}, 10)}
	origins := []routing.Point{{Latitude: 54.6872, Longitude: 25.2797}}
	warmer := NewRoutingWarmer(context.Background(), discardLogger(), fakeAreaStore{}, recordingDriveTimes{origins: new([]routing.Point)}, server, origins)

	warmer.Warm()
	warmer.Warm()
	warmer.Warm()

	select {
	case <-server.asked:
	case <-time.After(2 * time.Second):
		t.Fatal("the routing server was never asked")
	}
	time.Sleep(50 * time.Millisecond)
	if got := len(server.asked); got != 0 {
		t.Errorf("the routing server was asked %d more times, want none", got)
	}
}

func TestRoutingWarmerTriesAgainAfterAFailure(t *testing.T) {
	t.Parallel()

	server := signalingDriveTimes{asked: make(chan struct{}, 10), err: errors.New("asleep")}
	origins := []routing.Point{{Latitude: 54.6872, Longitude: 25.2797}}
	warmer := NewRoutingWarmer(context.Background(), discardLogger(), fakeAreaStore{}, recordingDriveTimes{origins: new([]routing.Point)}, server, origins)

	deadline := time.After(2 * time.Second)
	for asked := 0; asked < 2; {
		warmer.Warm()
		select {
		case <-server.asked:
			asked++
		case <-deadline:
			t.Fatalf("the routing server was asked %d times, want 2", asked)
		case <-time.After(10 * time.Millisecond):
		}
	}
}
