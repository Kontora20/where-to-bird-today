// Package routing asks an OSRM server how long it takes to drive between
// places.
package routing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Unreachable is the drive time reported for a destination with no road
// route from the origin.
const Unreachable = time.Duration(-1)

// maxSnapMeters is how far OSRM may move a coordinate to reach a road.
// Without a limit it snaps any point to the nearest road however far away:
// an origin in the Baltic Sea or in Belarus gets a route from a road tens of
// kilometres off. An origin beyond it is refused, and a destination beyond it
// (an island or a ferry point) counts as unreachable by car.
const maxSnapMeters = 5000

// ErrOriginOffRoad means no road lies close enough to the origin to start a
// route from it.
var ErrOriginOffRoad = errors.New("no road near the origin")

// Point is a WGS 84 coordinate.
type Point struct {
	Latitude  float64
	Longitude float64
}

// Client talks to one OSRM server. The server must allow as many coordinates
// per table request as there are destinations (osrm-routed --max-table-size).
type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type tableResponse struct {
	Code         string       `json:"code"`
	Message      string       `json:"message"`
	Durations    [][]*float64 `json:"durations"`
	Destinations []struct {
		// Distance is how far the coordinate was moved to reach a road.
		Distance float64 `json:"distance"`
	} `json:"destinations"`
}

// DriveTimes returns the drive time from origin to each destination, in the
// order given. A destination with no route has the value Unreachable.
func (c *Client) DriveTimes(ctx context.Context, origin Point, destinations []Point) ([]time.Duration, error) {
	if len(destinations) == 0 {
		return nil, nil
	}

	coordinates := make([]string, 0, len(destinations)+1)
	for _, point := range append([]Point{origin}, destinations...) {
		coordinates = append(coordinates,
			strconv.FormatFloat(point.Longitude, 'f', 6, 64)+","+strconv.FormatFloat(point.Latitude, 'f', 6, 64))
	}

	// Only the origin gets a snap limit here: OSRM fails the whole table when
	// any limited coordinate has no road in range, and one far destination
	// must not take down the rest. Destinations are checked after the answer.
	radiuses := strconv.Itoa(maxSnapMeters) + strings.Repeat(";unlimited", len(destinations))

	requestURL := fmt.Sprintf("%s/table/v1/driving/%s?%s",
		c.baseURL, strings.Join(coordinates, ";"),
		url.Values{"sources": {"0"}, "annotations": {"duration"}, "radiuses": {radiuses}}.Encode())

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building OSRM request: %w", err)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("calling OSRM: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("reading OSRM response: %w", err)
	}

	// OSRM reports its own failures in the body, with a 400 status, so the
	// body is decoded before the status is judged.
	var table tableResponse
	if err := json.Unmarshal(body, &table); err != nil {
		return nil, fmt.Errorf("OSRM answered %d with a body that is not JSON: %w", response.StatusCode, err)
	}
	if table.Code != "Ok" {
		if table.Code == "NoSegment" && strings.HasSuffix(table.Message, "coordinate 0") {
			return nil, ErrOriginOffRoad
		}
		return nil, fmt.Errorf("OSRM answered %d %s: %s", response.StatusCode, table.Code, table.Message)
	}
	if len(table.Durations) != 1 || len(table.Durations[0]) != len(destinations)+1 || len(table.Destinations) != len(destinations)+1 {
		return nil, errors.New("OSRM returned a table of the wrong size")
	}

	times := make([]time.Duration, len(destinations))
	for i, seconds := range table.Durations[0][1:] {
		if seconds == nil || table.Destinations[i+1].Distance > maxSnapMeters {
			times[i] = Unreachable
			continue
		}
		times[i] = time.Duration(*seconds * float64(time.Second))
	}
	return times, nil
}
