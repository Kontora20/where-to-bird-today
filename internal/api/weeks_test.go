package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/Kontora20/where-to-bird-today/internal/birding"
)

func TestGetAreaWeeks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		query      string
		service    fakeWeekFinder
		wantStatus int
	}{
		{
			name:       "by radius",
			query:      "lat=54.7&lon=25.3&radius_km=30&species=10&date=2026-05-15",
			service:    fakeWeekFinder{weeks: birding.AreaWeeks{Week: 20}},
			wantStatus: http.StatusOK,
		},
		{
			name:       "by drive time",
			query:      "lat=54.7&lon=25.3&max_minutes=45&species=10&species=11",
			wantStatus: http.StatusOK,
		},
		{name: "no species", query: "lat=54.7&lon=25.3&radius_km=30", wantStatus: http.StatusBadRequest},
		{name: "bad latitude", query: "lat=95&lon=25.3&radius_km=30&species=10", wantStatus: http.StatusBadRequest},
		{name: "no radius or drive time", query: "lat=54.7&lon=25.3&species=10", wantStatus: http.StatusBadRequest},
		{name: "bad date", query: "lat=54.7&lon=25.3&radius_km=30&species=10&date=15.05.2026", wantStatus: http.StatusBadRequest},
		{name: "bad species", query: "lat=54.7&lon=25.3&radius_km=30&species=owl", wantStatus: http.StatusBadRequest},
		{
			name:       "no routing server",
			query:      "lat=54.7&lon=25.3&max_minutes=45&species=10",
			service:    fakeWeekFinder{weeksErr: birding.ErrTravelTimeUnavailable},
			wantStatus: http.StatusServiceUnavailable,
		},
		{
			name:       "no road near the point",
			query:      "lat=54.7&lon=25.3&max_minutes=45&species=10",
			service:    fakeWeekFinder{weeksErr: birding.ErrNoRoadNearOrigin},
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:       "service error",
			query:      "lat=54.7&lon=25.3&radius_km=30&species=10",
			service:    fakeWeekFinder{weeksErr: errors.New("db down")},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/recommendations/weeks?"+tc.query, nil)
			rec := httptest.NewRecorder()

			handleGetAreaWeeks(testLogger(), tc.service).ServeHTTP(rec, req)

			if got := rec.Code; got != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %q)", got, tc.wantStatus, rec.Body.String())
			}
			if tc.wantStatus == http.StatusOK {
				var got birding.AreaWeeks
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatalf("body is not valid JSON: %v (body: %q)", err, rec.Body.String())
				}
			}
		})
	}
}

func TestGetAreaWeeksPassesTheSearchOn(t *testing.T) {
	t.Parallel()

	var got birding.AreaParams
	service := fakeWeekFinder{gotAreaParams: &got}
	req := httptest.NewRequest(http.MethodGet, "/recommendations/weeks?lat=54.7&lon=25.3&max_minutes=45&species=10&species=11&date=2026-05-15", nil)
	rec := httptest.NewRecorder()

	handleGetAreaWeeks(testLogger(), service).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %q)", rec.Code, rec.Body.String())
	}
	if got.Latitude != 54.7 || got.Longitude != 25.3 || got.MaxMinutes != 45 || got.RadiusKM != 0 ||
		!slices.Equal(got.SpeciesIDs, []int64{10, 11}) || got.Date.Format("2006-01-02") != "2026-05-15" {
		t.Errorf("AreaWeeks got %+v, want the point, 45 minutes, species 10 and 11 and 2026-05-15", got)
	}
}
