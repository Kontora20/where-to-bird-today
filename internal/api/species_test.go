package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Kontora20/where-to-bird-today/internal/birding"
)

func TestGetSpeciesDetail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		hotspotID  string
		speciesID  string
		service    fakeSpeciesDetailer
		wantStatus int
	}{
		{
			name:       "found",
			hotspotID:  "1",
			speciesID:  "10",
			service:    fakeSpeciesDetailer{speciesFound: birding.SpeciesDetail{SpeciesID: 10, CommonName: "White Stork"}},
			wantStatus: http.StatusOK,
		},
		{
			name:       "species not found",
			hotspotID:  "1",
			speciesID:  "404",
			service:    fakeSpeciesDetailer{speciesErr: pgx.ErrNoRows},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "non-numeric hotspotId",
			hotspotID:  "not-a-number",
			speciesID:  "10",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "non-numeric speciesId",
			hotspotID:  "1",
			speciesID:  "not-a-number",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "service error",
			hotspotID:  "1",
			speciesID:  "10",
			service:    fakeSpeciesDetailer{speciesErr: errors.New("db down")},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/hotspots/"+tc.hotspotID+"/species/"+tc.speciesID, nil)
			req.SetPathValue("hotspotId", tc.hotspotID)
			req.SetPathValue("speciesId", tc.speciesID)
			rec := httptest.NewRecorder()

			handleGetSpeciesDetail(testLogger(), tc.service).ServeHTTP(rec, req)

			if got := rec.Code; got != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %q)", got, tc.wantStatus, rec.Body.String())
			}

			if tc.wantStatus == http.StatusOK {
				var got birding.SpeciesDetail
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatalf("body is not valid JSON: %v (body: %q)", err, rec.Body.String())
				}
				if got.CommonName != "White Stork" {
					t.Errorf("body.common_name = %q, want %q", got.CommonName, "White Stork")
				}
			}
		})
	}
}

func TestGetSpeciesDetailDate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		query      string
		wantStatus int
		wantDate   string
	}{
		{name: "date is passed on", query: "?date=2026-06-21", wantStatus: http.StatusOK, wantDate: "2026-06-21"},
		{name: "malformed date", query: "?date=21.06.2026", wantStatus: http.StatusBadRequest},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var gotDate time.Time
			service := fakeSpeciesDetailer{gotDate: &gotDate}
			req := httptest.NewRequest(http.MethodGet, "/hotspots/1/species/10"+tc.query, nil)
			req.SetPathValue("hotspotId", "1")
			req.SetPathValue("speciesId", "10")
			rec := httptest.NewRecorder()

			handleGetSpeciesDetail(testLogger(), service).ServeHTTP(rec, req)

			if got := rec.Code; got != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %q)", got, tc.wantStatus, rec.Body.String())
			}
			if tc.wantDate != "" && gotDate.Format("2006-01-02") != tc.wantDate {
				t.Errorf("service got date %s, want %s", gotDate.Format("2006-01-02"), tc.wantDate)
			}
		})
	}
}

func TestSearchSpecies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		query      string
		service    fakeSpeciesSearcher
		wantStatus int
	}{
		{
			name:       "found",
			query:      "q=stork",
			service:    fakeSpeciesSearcher{searchFound: []birding.SpeciesMatch{{SpeciesID: 10, CommonName: "White Stork"}}},
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing q",
			query:      "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "blank q",
			query:      "q=+++",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "service error",
			query:      "q=stork",
			service:    fakeSpeciesSearcher{searchErr: errors.New("db down")},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/species?"+tc.query, nil)
			rec := httptest.NewRecorder()

			handleSearchSpecies(testLogger(), tc.service).ServeHTTP(rec, req)

			if got := rec.Code; got != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %q)", got, tc.wantStatus, rec.Body.String())
			}
		})
	}
}
