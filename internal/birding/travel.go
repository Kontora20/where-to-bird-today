package birding

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Kontora20/where-to-bird-today/internal/routing"
)

// maxSpeedKMPerMinute is 130 km/h, the fastest legal road speed in the
// Baltics. No route covers more straight-line distance than this in a given
// time, so hotspots beyond it can be dropped before asking for drive times.
const maxSpeedKMPerMinute = 130.0 / 60

// MaxDriveMinutes bounds a search's ?max_minutes=. Matches the UI's largest
// choice: a higher limit only widens the PostGIS radius and the OSRM table
// request (master plan row 31) for a search nobody's search box can ask for.
// Exported, and the one definition of the bound, so the API's validation and
// the drive-time cache's warm-up (row 30) can't drift apart the way the
// bound itself already has once (240 minutes down to this).
const MaxDriveMinutes = 120

// MaxRadiusKM bounds a search's ?radius_km=. A radius wider than the country
// only makes the PostGIS query return every hotspot there is.
const MaxRadiusKM = 300

// ErrTravelTimeUnavailable means the request asked for a drive time limit
// but no routing server is configured.
var ErrTravelTimeUnavailable = errors.New("travel time is not available")

// ErrNoRoadNearOrigin means no road lies close enough to the search point to
// start a route from it.
var ErrNoRoadNearOrigin = routing.ErrOriginOffRoad

// ErrRoutingBusy means the routing server had no free capacity for this
// request.
var ErrRoutingBusy = routing.ErrBusy

// DriveTimeFinder measures the drive time from one point to many.
type DriveTimeFinder interface {
	DriveTimes(ctx context.Context, origin routing.Point, destinations []routing.Point) ([]time.Duration, error)
}

// travel limits a search of an area by distance or by drive time. driveTimes
// may be nil, in which case a limit by drive time fails with
// ErrTravelTimeUnavailable.
type travel struct {
	driveTimes DriveTimeFinder
}

// searchRadiusKM is how far from the point to look for hotspots. A drive time
// limit replaces the radius, at the fastest speed any road allows, and needs a
// routing server.
func (t travel) searchRadiusKM(radiusKM, maxMinutes float64) (float64, error) {
	if maxMinutes <= 0 {
		return radiusKM, nil
	}
	if t.driveTimes == nil {
		return 0, ErrTravelTimeUnavailable
	}
	return maxMinutes * maxSpeedKMPerMinute, nil
}

// reachable returns the positions in destinations that are within maxMinutes of
// driving from origin, in order, and the drive time to each of them.
func (t travel) reachable(ctx context.Context, origin routing.Point, destinations []routing.Point, maxMinutes float64) ([]int, []time.Duration, error) {
	all, err := t.driveTimes.DriveTimes(ctx, origin, destinations)
	if err != nil {
		return nil, nil, fmt.Errorf("finding drive times: %w", err)
	}

	limit := time.Duration(maxMinutes * float64(time.Minute))
	var positions []int
	var times []time.Duration
	for i, driveTime := range all {
		if driveTime == routing.Unreachable || driveTime > limit {
			continue
		}
		positions = append(positions, i)
		times = append(times, driveTime)
	}
	return positions, times, nil
}
