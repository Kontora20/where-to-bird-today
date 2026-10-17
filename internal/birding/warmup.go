package birding

import (
	"context"
	"log/slog"
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
// asks. Meant to run once at startup, in its own goroutine: a slow or failed
// routing server must not delay the server coming up, and a real search
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
