package routing

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"
)

// maxCachedOrigins bounds the memory of a Cache. Anyone may search from any
// point, so without a bound every new start point would add to it for good.
// One origin holds at most one entry per hotspot.
const maxCachedOrigins = 100

// originScale rounds a start point to three decimals, about 100 m. That is far
// below the minute a drive time is rounded up to, and it lets searches from
// neighbouring points share their answers.
const originScale = 1000

// driveTimeFinder is what a Cache needs from the server behind it. *Client
// satisfies it.
type driveTimeFinder interface {
	DriveTimes(ctx context.Context, origin Point, destinations []Point) ([]time.Duration, error)
}

// Cache remembers the drive times it has been given, so a start point that was
// searched before costs no request to the routing server. Only destinations
// not yet known from that start point are asked for. The road data does not
// change while the server runs, so an entry never goes stale.
//
// The start point must be a real coordinate: NaN is never equal to itself, so
// it could not be found again. The API rejects it before a search starts.
type Cache struct {
	next driveTimeFinder

	mu      sync.Mutex
	origins map[Point]*originTimes
}

// originTimes holds what is known from one start point.
type originTimes struct {
	// mu is held for a whole lookup, including the request to the server. A
	// second search from the same start point waits for the first and then
	// finds its answers here instead of asking again.
	mu    sync.Mutex
	times map[Point]time.Duration
}

// NewCache returns a Cache that asks next for the drive times it does not know.
func NewCache(next driveTimeFinder) *Cache {
	return &Cache{next: next, origins: make(map[Point]*originTimes)}
}

// DriveTimes returns the drive time from origin, rounded to about 100 m, to
// each destination, in the order given. A destination with no route has the
// value Unreachable. A failed request is not remembered.
func (c *Cache) DriveTimes(ctx context.Context, origin Point, destinations []Point) ([]time.Duration, error) {
	if len(destinations) == 0 {
		return nil, nil
	}
	origin = roundOrigin(origin)
	known := c.originTimes(origin)
	known.mu.Lock()
	defer known.mu.Unlock()

	var missing []Point
	for _, destination := range destinations {
		if _, ok := known.times[destination]; !ok {
			missing = append(missing, destination)
		}
	}
	if len(missing) > 0 {
		fetched, err := c.next.DriveTimes(ctx, origin, missing)
		if err != nil {
			return nil, err
		}
		if len(fetched) != len(missing) {
			return nil, fmt.Errorf("asked for %d drive times and got %d", len(missing), len(fetched))
		}
		for i, destination := range missing {
			known.times[destination] = fetched[i]
		}
	}

	times := make([]time.Duration, len(destinations))
	for i, destination := range destinations {
		times[i] = known.times[destination]
	}
	return times, nil
}

// originTimes returns what is known from origin, making room for it first when
// the cache is full.
func (c *Cache) originTimes(origin Point) *originTimes {
	c.mu.Lock()
	defer c.mu.Unlock()

	known, ok := c.origins[origin]
	if ok {
		return known
	}
	if len(c.origins) >= maxCachedOrigins {
		// ponytail: drops an arbitrary origin; keep the order of use and drop
		// the least recently used one if the hit rate matters.
		for evicted := range c.origins {
			delete(c.origins, evicted)
			break
		}
	}
	known = &originTimes{times: make(map[Point]time.Duration)}
	c.origins[origin] = known
	return known
}

func roundOrigin(origin Point) Point {
	return Point{
		Latitude:  math.Round(origin.Latitude*originScale) / originScale,
		Longitude: math.Round(origin.Longitude*originScale) / originScale,
	}
}
