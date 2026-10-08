package routing

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"
)

// fakeFinder answers with one minute per degree of latitude of the destination
// and records what it was asked. It takes a moment to answer, so that searches
// started together overlap.
type fakeFinder struct {
	mu      sync.Mutex
	origins []Point
	asked   [][]Point
	err     error
}

func (f *fakeFinder) DriveTimes(_ context.Context, origin Point, destinations []Point) ([]time.Duration, error) {
	time.Sleep(5 * time.Millisecond)

	f.mu.Lock()
	defer f.mu.Unlock()
	f.origins = append(f.origins, origin)
	f.asked = append(f.asked, slices.Clone(destinations))
	if f.err != nil {
		return nil, f.err
	}
	times := make([]time.Duration, len(destinations))
	for i, destination := range destinations {
		times[i] = minutesTo(destination)
	}
	return times, nil
}

func point(n float64) Point { return Point{Latitude: n, Longitude: n} }

func minutesTo(destination Point) time.Duration {
	return time.Duration(destination.Latitude) * time.Minute
}

func TestCacheAsksOnlyForUnknownDestinations(t *testing.T) {
	t.Parallel()

	steps := []struct {
		name         string
		destinations []Point
		wantAsked    []Point // nil: the server is not called
	}{
		{name: "first search asks for all", destinations: []Point{point(1), point(2)}, wantAsked: []Point{point(1), point(2)}},
		{name: "the same search asks for nothing", destinations: []Point{point(1), point(2)}},
		{name: "a wider search asks for the new one", destinations: []Point{point(2), point(3)}, wantAsked: []Point{point(3)}},
		{name: "answers follow the order asked", destinations: []Point{point(3), point(1), point(2)}},
	}

	finder := &fakeFinder{}
	cache := NewCache(finder)
	origin := Point{Latitude: 54.687, Longitude: 25.28}
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			finder.asked = nil
			got, err := cache.DriveTimes(context.Background(), origin, step.destinations)
			if err != nil {
				t.Fatalf("DriveTimes() error = %v", err)
			}
			want := make([]time.Duration, len(step.destinations))
			for i, destination := range step.destinations {
				want[i] = minutesTo(destination)
			}
			if !slices.Equal(got, want) {
				t.Errorf("DriveTimes() = %v, want %v", got, want)
			}
			var asked []Point
			if len(finder.asked) > 0 {
				asked = finder.asked[0]
			}
			if !slices.Equal(asked, step.wantAsked) {
				t.Errorf("server was asked for %v, want %v", asked, step.wantAsked)
			}
		})
	}
}

func TestCacheSharesNeighbouringOrigins(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		origin     Point
		wantCalled bool
	}{
		{name: "about 30 m away", origin: Point{Latitude: 54.68749, Longitude: 25.27951}},
		{name: "several km away", origin: Point{Latitude: 54.75, Longitude: 25.28}, wantCalled: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			finder := &fakeFinder{}
			cache := NewCache(finder)
			destinations := []Point{point(1)}
			if _, err := cache.DriveTimes(context.Background(), Point{Latitude: 54.68721, Longitude: 25.27971}, destinations); err != nil {
				t.Fatalf("first DriveTimes() error = %v", err)
			}
			if _, err := cache.DriveTimes(context.Background(), tc.origin, destinations); err != nil {
				t.Fatalf("second DriveTimes() error = %v", err)
			}

			if got := len(finder.asked) == 2; got != tc.wantCalled {
				t.Errorf("server called again = %t, want %t", got, tc.wantCalled)
			}
			if want := (Point{Latitude: 54.687, Longitude: 25.28}); finder.origins[0] != want {
				t.Errorf("server was asked from %v, want the rounded %v", finder.origins[0], want)
			}
		})
	}
}

func TestCacheDoesNotRememberAFailure(t *testing.T) {
	t.Parallel()

	finder := &fakeFinder{err: ErrOriginOffRoad}
	cache := NewCache(finder)
	origin := Point{Latitude: 54.687, Longitude: 25.28}
	destinations := []Point{point(1)}

	if _, err := cache.DriveTimes(context.Background(), origin, destinations); !errors.Is(err, ErrOriginOffRoad) {
		t.Fatalf("DriveTimes() error = %v, want %v", err, ErrOriginOffRoad)
	}
	finder.err = nil
	got, err := cache.DriveTimes(context.Background(), origin, destinations)
	if err != nil {
		t.Fatalf("DriveTimes() after the failure: error = %v", err)
	}
	if want := []time.Duration{time.Minute}; !slices.Equal(got, want) {
		t.Errorf("DriveTimes() = %v, want %v", got, want)
	}
	if len(finder.asked) != 2 {
		t.Errorf("server was called %d times, want 2", len(finder.asked))
	}
}

func TestCacheStaysWithinItsLimit(t *testing.T) {
	t.Parallel()

	cache := NewCache(&fakeFinder{})
	for i := range maxCachedOrigins + 20 {
		origin := Point{Latitude: 54 + float64(i)/100, Longitude: 25}
		if _, err := cache.DriveTimes(context.Background(), origin, []Point{point(1)}); err != nil {
			t.Fatalf("DriveTimes() from origin %d: error = %v", i, err)
		}
	}
	if got := len(cache.origins); got != maxCachedOrigins {
		t.Errorf("cache holds %d origins, want %d", got, maxCachedOrigins)
	}
}

func TestCacheAsksOnceForSearchesStartedTogether(t *testing.T) {
	t.Parallel()

	finder := &fakeFinder{}
	cache := NewCache(finder)
	origin := Point{Latitude: 54.687, Longitude: 25.28}
	destinations := []Point{point(1), point(2)}

	var group sync.WaitGroup
	for range 8 {
		group.Go(func() {
			if _, err := cache.DriveTimes(context.Background(), origin, destinations); err != nil {
				t.Errorf("DriveTimes() error = %v", err)
			}
		})
	}
	group.Wait()

	if len(finder.asked) != 1 {
		t.Errorf("server was called %d times, want 1", len(finder.asked))
	}
}

// blockingFinder answers only once release is closed, so a test can hold a
// Cache's lock open for as long as it needs to.
type blockingFinder struct {
	release chan struct{}
}

func (b blockingFinder) DriveTimes(_ context.Context, _ Point, destinations []Point) ([]time.Duration, error) {
	<-b.release
	return make([]time.Duration, len(destinations)), nil
}

func TestCacheRespectsCancellationWhileWaitingForTheLock(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	cache := NewCache(blockingFinder{release: release})
	origin := Point{Latitude: 54.687, Longitude: 25.28}
	destinations := []Point{point(1)}

	// Starts a search that acquires the lock and then blocks inside the call
	// to the server, so the lock stays held until release is closed below.
	go func() { _, _ = cache.DriveTimes(context.Background(), origin, destinations) }()
	time.Sleep(5 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := cache.DriveTimes(ctx, origin, destinations); !errors.Is(err, context.Canceled) {
		t.Errorf("DriveTimes() with a cancelled context while the lock is held: error = %v, want %v", err, context.Canceled)
	}

	close(release)
}

func TestCacheWithNoDestinationsSkipsTheServer(t *testing.T) {
	t.Parallel()

	finder := &fakeFinder{}
	got, err := NewCache(finder).DriveTimes(context.Background(), Point{}, nil)
	if err != nil || got != nil {
		t.Errorf("DriveTimes() = %v, %v, want nil, nil", got, err)
	}
	if len(finder.asked) != 0 {
		t.Errorf("server was called %d times, want 0", len(finder.asked))
	}
}

// searchedFrom reports whether the server had to be asked about origin: a
// search from an origin the cache holds costs no request.
func searchedFrom(t *testing.T, cache *Cache, finder *fakeFinder, origin Point) bool {
	t.Helper()

	before := len(finder.asked)
	if _, err := cache.DriveTimes(context.Background(), origin, []Point{point(1)}); err != nil {
		t.Fatalf("DriveTimes() error = %v", err)
	}
	return len(finder.asked) > before
}

// farOrigin is the nth start point in a line a degree apart, so none of them
// round to the same one.
func farOrigin(n int) Point { return Point{Latitude: 50 + float64(n)*0.01, Longitude: 20} }

func TestCacheDropsTheLeastRecentlyUsedOrigin(t *testing.T) {
	t.Parallel()

	finder := &fakeFinder{}
	cache := NewCache(finder)
	for n := range maxCachedOrigins {
		searchedFrom(t, cache, finder, farOrigin(n))
	}

	// Origin 0 was searched first, but searching it again makes origin 1 the
	// one used longest ago.
	if searchedFrom(t, cache, finder, farOrigin(0)) {
		t.Fatal("origin 0 was asked for again before the cache was full")
	}
	searchedFrom(t, cache, finder, farOrigin(maxCachedOrigins))

	if searchedFrom(t, cache, finder, farOrigin(0)) {
		t.Error("origin 0 was dropped although it was used most recently")
	}
	if !searchedFrom(t, cache, finder, farOrigin(1)) {
		t.Error("origin 1 was kept although it was used longest ago")
	}
}

func TestCacheKeepsPinnedOrigins(t *testing.T) {
	t.Parallel()

	pinned := Point{Latitude: 54.6872, Longitude: 25.2797}
	finder := &fakeFinder{}
	cache := NewCache(finder, pinned)
	searchedFrom(t, cache, finder, pinned)

	for n := range 3 * maxCachedOrigins {
		searchedFrom(t, cache, finder, farOrigin(n))
	}

	if searchedFrom(t, cache, finder, pinned) {
		t.Error("the pinned origin was dropped by searches from other origins")
	}
}
