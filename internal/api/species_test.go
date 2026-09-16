package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/Kontora20/where-to-bird-today/internal/birding"
)

func TestGetSpeciesDetail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		hotspotID  string
		speciesID  string
		service    fakeRecommendationService
		wantStatus int
	}{
		{
			name:       "found",
			hotspotID:  "1",
			speciesID:  "10",
			service:    fakeRecommendationService{speciesFound: birding.SpeciesDetail{SpeciesID: 10, CommonName: "White Stork"}},
			wantStatus: http.StatusOK,
		},
		{
			name:       "species not found",
			hotspotID:  "1",
			speciesID:  "404",
			service:    fakeRecommendationService{speciesErr: pgx.ErrNoRows},
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
			service:    fakeRecommendationService{speciesErr: errors.New("db down")},
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
