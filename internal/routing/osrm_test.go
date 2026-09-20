package routing

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestDriveTimes(t *testing.T) {
	t.Parallel()

	origin := Point{Latitude: 54.6872, Longitude: 25.2797}
	destinations := []Point{
		{Latitude: 55.3167, Longitude: 21.3667},
		{Latitude: 54.1, Longitude: 24.5},
	}

	tests := []struct {
		name       string
		status     int
		body       string
		want       []time.Duration
		wantErr    error
		wantAnyErr bool
	}{
		{
			name:   "durations in destination order",
			status: http.StatusOK,
			body:   `{"code":"Ok","durations":[[0,90.5,1800]],"destinations":[{"distance":3},{"distance":40},{"distance":5000}]}`,
			want:   []time.Duration{90*time.Second + 500*time.Millisecond, 30 * time.Minute},
		},
		{
			name:   "a null duration is unreachable",
			status: http.StatusOK,
			body:   `{"code":"Ok","durations":[[0,null,1800]],"destinations":[{"distance":3},{"distance":40},{"distance":40}]}`,
			want:   []time.Duration{Unreachable, 30 * time.Minute},
		},
		{
			name:   "a destination too far from any road is unreachable",
			status: http.StatusOK,
			body:   `{"code":"Ok","durations":[[0,90,1800]],"destinations":[{"distance":3},{"distance":5000.5},{"distance":40}]}`,
			want:   []time.Duration{Unreachable, 30 * time.Minute},
		},
		{
			name:    "no road near the origin",
			status:  http.StatusBadRequest,
			body:    `{"code":"NoSegment","message":"Could not find a matching segment for coordinate 0"}`,
			wantErr: ErrOriginOffRoad,
		},
		{
			name:       "no road near a destination is a plain failure",
			status:     http.StatusBadRequest,
			body:       `{"code":"NoSegment","message":"Could not find a matching segment for coordinate 2"}`,
			wantAnyErr: true,
		},
		{
			name:       "table too big",
			status:     http.StatusBadRequest,
			body:       `{"code":"TooBig","message":"Too many table coordinates"}`,
			wantAnyErr: true,
		},
		{
			name:       "table of the wrong size",
			status:     http.StatusOK,
			body:       `{"code":"Ok","durations":[[0,90]],"destinations":[{"distance":3},{"distance":40}]}`,
			wantAnyErr: true,
		},
		{
			name:       "answer without snap distances",
			status:     http.StatusOK,
			body:       `{"code":"Ok","durations":[[0,90,1800]]}`,
			wantAnyErr: true,
		},
		{
			name:       "body that is not JSON",
			status:     http.StatusBadGateway,
			body:       `<html>bad gateway</html>`,
			wantAnyErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var gotPath, gotQuery string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(server.Close)

			got, err := NewClient(server.URL+"/").DriveTimes(context.Background(), origin, destinations)

			wantPath := "/table/v1/driving/25.279700,54.687200;21.366700,55.316700;24.500000,54.100000"
			const wantQuery = "annotations=duration&radiuses=5000%3Bunlimited%3Bunlimited&sources=0"
			if gotPath != wantPath || gotQuery != wantQuery {
				t.Errorf("requested %s?%s, want %s?%s", gotPath, gotQuery, wantPath, wantQuery)
			}
			switch {
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("DriveTimes() error = %v, want %v", err, tc.wantErr)
				}
			case tc.wantAnyErr:
				if err == nil {
					t.Fatal("DriveTimes() error = nil, want an error")
				}
			default:
				if err != nil {
					t.Fatalf("DriveTimes() error = %v", err)
				}
				if !slices.Equal(got, tc.want) {
					t.Errorf("DriveTimes() = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestDriveTimesWithNoDestinationsSkipsTheServer(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("server was called for an empty destination list")
	}))
	t.Cleanup(server.Close)

	got, err := NewClient(server.URL).DriveTimes(context.Background(), Point{}, nil)
	if err != nil || got != nil {
		t.Errorf("DriveTimes() = %v, %v, want nil, nil", got, err)
	}
}

func TestDriveTimesReportsTransportFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	baseURL := server.URL
	server.Close()

	_, err := NewClient(baseURL).DriveTimes(context.Background(), Point{}, []Point{{Latitude: 1, Longitude: 1}})
	if err == nil || !strings.Contains(err.Error(), "calling OSRM") {
		t.Errorf("DriveTimes() error = %v, want one mentioning \"calling OSRM\"", err)
	}
}
