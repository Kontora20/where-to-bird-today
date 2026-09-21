package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Kontora20/where-to-bird-today/internal/birding"
)

func TestGetInSeason(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		query      string
		service    fakeSeasonFinder
		wantStatus int
	}{
		{name: "by drive time", query: "lat=54.7&lon=25.3&max_minutes=45&date=2026-09-16", wantStatus: http.StatusOK},
		{name: "by radius", query: "lat=54.7&lon=25.3&radius_km=30", wantStatus: http.StatusOK},
		{name: "a bad species value is rejected", query: "lat=54.7&lon=25.3&radius_km=30&species=owl", wantStatus: http.StatusBadRequest},
		{name: "bad latitude", query: "lat=95&lon=25.3&radius_km=30", wantStatus: http.StatusBadRequest},
		{name: "no radius or drive time", query: "lat=54.7&lon=25.3", wantStatus: http.StatusBadRequest},
		{name: "bad date", query: "lat=54.7&lon=25.3&radius_km=30&date=16.09.2026", wantStatus: http.StatusBadRequest},
		{
			name:       "no routing server",
			query:      "lat=54.7&lon=25.3&max_minutes=45",
			service:    fakeSeasonFinder{seasonErr: birding.ErrTravelTimeUnavailable},
			wantStatus: http.StatusServiceUnavailable,
		},
		{
			name:       "no road near the point",
			query:      "lat=54.7&lon=25.3&max_minutes=45",
			service:    fakeSeasonFinder{seasonErr: birding.ErrNoRoadNearOrigin},
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:       "service error",
			query:      "lat=54.7&lon=25.3&radius_km=30",
			service:    fakeSeasonFinder{seasonErr: errors.New("db down")},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/in-season?"+tc.query, nil)
			rec := httptest.NewRecorder()

			handleGetInSeason(testLogger(), tc.service).ServeHTTP(rec, req)

			if got := rec.Code; got != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %q)", got, tc.wantStatus, rec.Body.String())
			}
			if tc.wantStatus == http.StatusOK {
				var got birding.Season
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatalf("body is not valid JSON: %v (body: %q)", err, rec.Body.String())
				}
			}
		})
	}
}

func TestGetInSeasonPassesTheSearchOn(t *testing.T) {
	t.Parallel()

	var gotParams birding.AreaParams
	var gotLang string
	service := fakeSeasonFinder{gotAreaParams: &gotParams, gotLang: &gotLang}
	req := httptest.NewRequest(http.MethodGet, "/in-season?lat=54.7&lon=25.3&max_minutes=45&date=2026-09-16&lang=lt", nil)
	rec := httptest.NewRecorder()

	handleGetInSeason(testLogger(), service).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %q)", rec.Code, rec.Body.String())
	}
	if gotParams.Latitude != 54.7 || gotParams.Longitude != 25.3 || gotParams.MaxMinutes != 45 || gotParams.RadiusKM != 0 ||
		gotParams.Date.Format("2006-01-02") != "2026-09-16" {
		t.Errorf("InSeason got %+v, want the point, 45 minutes and 2026-09-16", gotParams)
	}
	if gotLang != "lt" {
		t.Errorf("InSeason got language %q, want lt", gotLang)
	}
}
