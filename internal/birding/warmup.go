package birding

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/Kontora20/where-to-bird-today/internal/routing"
	"github.com/Kontora20/where-to-bird-today/internal/store"
)

// warmupRadiusMeters is exactly how far a real search's drive-time limit can
// ever reach (MaxDriveMinutes at the fastest legal road speed), so the
// warm-up asks for precisely the hotspots a real search from origin could
// ask for -- no more, no less. Tied to MaxDriveMinutes rather than a
// separate guessed number, so the two can't drift apart the way the bound
// itself already has once.
const warmupRadiusMeters = MaxDriveMinutes * maxSpeedKMPerMinute * 1000

// hotspotLister is the one query WarmDriveTimeCache needs.
type hotspotLister interface {
	ListHotspotsWithin(ctx context.Context, arg store.ListHotspotsWithinParams) ([]store.ListHotspotsWithinRow, error)
}

// WarmDriveTimeCache asks driveTimes for the drive time from each of origins
// to every hotspot within reach of it, so a cache sitting in front of
// driveTimes (routing.NewCache) already knows them before a real search
// asks. Meant to run in its own goroutine, started by a RoutingWarmer: a slow or failed
// routing server must not hold anything up, and a real search
// still falls back to asking driveTimes directly either way (master plan
// row 30). origins is typically the UI's default start point plus a handful
// of cities real searches cluster around -- each one is cheap (one drive-time
// request) and origins is far below routing.Cache's 100-origin limit.
func WarmDriveTimeCache(ctx context.Context, logger *slog.Logger, hotspots hotspotLister, driveTimes DriveTimeFinder, origins []routing.Point) {
	for _, origin := range origins {
		warmOneOrigin(ctx, logger, hotspots, driveTimes, origin)
	}
}

func warmOneOrigin(ctx context.Context, logger *slog.Logger, hotspots hotspotLister, driveTimes DriveTimeFinder, origin routing.Point) {
	rows, err := hotspots.ListHotspotsWithin(ctx, store.ListHotspotsWithinParams{
		Latitude:     origin.Latitude,
		Longitude:    origin.Longitude,
		RadiusMeters: warmupRadiusMeters,
	})
	if err != nil {
		logger.Warn("drive-time cache warm-up: listing hotspots failed", slog.Float64("latitude", origin.Latitude), slog.Float64("longitude", origin.Longitude), slog.Any("error", err))
		return
	}

	destinations := make([]routing.Point, len(rows))
	for i, row := range rows {
		destinations[i] = routing.Point{Latitude: row.Latitude.Float64, Longitude: row.Longitude.Float64}
	}

	started := time.Now()
	if _, err := driveTimes.DriveTimes(ctx, origin, destinations); err != nil {
		logger.Warn("drive-time cache warm-up: asking the routing server failed", slog.Float64("latitude", origin.Latitude), slog.Float64("longitude", origin.Longitude), slog.Duration("took", time.Since(started)), slog.Any("error", err))
		return
	}
	logger.Info("drive-time cache warmed", slog.Float64("latitude", origin.Latitude), slog.Float64("longitude", origin.Longitude), slog.Int("hotspots", len(destinations)), slog.Duration("took", time.Since(started)))
}

// warmRepeatAfter is how soon after one warm-up another may start. The routing
// server scales to zero after about five minutes without a request, so a
// warm-up that is older than that may have a sleeping server to wake again.
const warmRepeatAfter = 5 * time.Minute

// RoutingWarmer wakes the routing server and fills the drive-time cache. It
// runs when a person asks for drive times, not when the server starts, so the
// routing server is only woken, and paid for, when someone wants it.
type RoutingWarmer struct {
	ctx        context.Context
	logger     *slog.Logger
	hotspots   hotspotLister
	driveTimes DriveTimeFinder
	server     DriveTimeFinder
	origins    []routing.Point

	mu        sync.Mutex
	lastStart time.Time
}

// NewRoutingWarmer returns a warmer that asks server directly to wake it, then
// warms driveTimes, the cache in front of it, for origins. ctx ends the work
// when the process shuts down.
func NewRoutingWarmer(ctx context.Context, logger *slog.Logger, hotspots hotspotLister, driveTimes, server DriveTimeFinder, origins []routing.Point) *RoutingWarmer {
	return &RoutingWarmer{ctx: ctx, logger: logger, hotspots: hotspots, driveTimes: driveTimes, server: server, origins: origins}
}

// Warm starts a warm-up in the background and returns at once. It does nothing
// when one started less than warmRepeatAfter ago, so many people asking in a
// row cost one warm-up.
func (w *RoutingWarmer) Warm() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.origins) == 0 || (!w.lastStart.IsZero() && time.Since(w.lastStart) < warmRepeatAfter) {
		return
	}
	w.lastStart = time.Now()
	go w.run()
}

func (w *RoutingWarmer) run() {
	// A cached city is answered from memory, so only a request to the routing
	// server itself wakes it: one drive time from a city to itself.
	started := time.Now()
	origin := w.origins[0]
	if _, err := w.server.DriveTimes(w.ctx, origin, []routing.Point{origin}); err != nil {
		w.logger.Warn("waking the routing server failed", slog.Duration("took", time.Since(started)), slog.Any("error", err))
		// Let the next person asking try again at once.
		w.mu.Lock()
		w.lastStart = time.Time{}
		w.mu.Unlock()
		return
	}
	w.logger.Info("routing server awake", slog.Duration("took", time.Since(started)))
	WarmDriveTimeCache(w.ctx, w.logger, w.hotspots, w.driveTimes, w.origins)
}
