package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/Kontora20/where-to-bird-today/internal/birding"
)

type fakeRecommendationService struct {
	err          error
	results      []birding.Recommendation
	speciesErr   error
	speciesFound birding.SpeciesDetail
	searchErr    error
	searchFound  []birding.SpeciesMatch
	// gotParams is a pointer because the methods have value receivers: a
	// plain field would be written to a copy.
	gotParams *birding.Params
}

func (f fakeRecommendationService) Recommend(_ context.Context, params birding.Params) ([]birding.Recommendation, error) {
	if f.gotParams != nil {
		*f.gotParams = params
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.results, nil
}

func (f fakeRecommendationService) SearchSpecies(_ context.Context, _, _ string) ([]birding.SpeciesMatch, error) {
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	return f.searchFound, nil
}

func (f fakeRecommendationService) SpeciesDetail(_ context.Context, _, _ int64, _ string) (birding.SpeciesDetail, error) {
	if f.speciesErr != nil {
		return birding.SpeciesDetail{}, f.speciesErr
	}
	return f.speciesFound, nil
}

func TestGetRecommendations(t *testing.T) {
	t.Parallel()

	validQuery := "lat=55.3167&lon=21.3667&radius_km=25"

	tests := []struct {
		name       string
		query      string
		service    fakeRecommendationService
		wantStatus int
	}{
		{
			name:       "valid request",
			query:      validQuery,
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing lat",
			query:      "lon=21.3667&radius_km=25",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "lat out of range",
			query:      "lat=95&lon=21.3667&radius_km=25",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "lon out of range",
			query:      "lat=55.3167&lon=200&radius_km=25",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing radius_km",
			query:      "lat=55.3167&lon=21.3667",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "zero radius_km",
			query:      "lat=55.3167&lon=21.3667&radius_km=0",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid date",
			query:      validQuery + "&date=not-a-date",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "valid date",
			query:      validQuery + "&date=2026-05-15",
			wantStatus: http.StatusOK,
		},
		{
			name:       "valid species",
			query:      validQuery + "&species=12&species=34",
			wantStatus: http.StatusOK,
		},
		{
			name:       "non-numeric species",
			query:      validQuery + "&species=crane",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "zero species",
			query:      validQuery + "&species=0",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "too many species",
			query:      validQuery + strings.Repeat("&species=1", maxTargetSpecies+1),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "service error",
			query:      validQuery,
			service:    fakeRecommendationService{err: errors.New("db down")},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/recommendations?"+tc.query, nil)
			rec := httptest.NewRecorder()

			handleGetRecommendations(testLogger(), tc.service).ServeHTTP(rec, req)

			if got := rec.Code; got != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %q)", got, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestGetRecommendationsPassesSpeciesIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query string
		want  []int64
	}{
		{name: "no species means all species", query: "", want: nil},
		{name: "repeated species values", query: "&species=12&species=34", want: []int64{12, 34}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var gotParams birding.Params
			req := httptest.NewRequest(http.MethodGet, "/recommendations?lat=55.3&lon=21.3&radius_km=25"+tc.query, nil)
			rec := httptest.NewRecorder()

			handleGetRecommendations(testLogger(), fakeRecommendationService{gotParams: &gotParams}).ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d (body: %q)", rec.Code, http.StatusOK, rec.Body.String())
			}
			if !slices.Equal(gotParams.SpeciesIDs, tc.want) {
				t.Errorf("SpeciesIDs = %v, want %v", gotParams.SpeciesIDs, tc.want)
			}
		})
	}
}

func TestResolveLanguage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		langParam      string
		acceptLanguage string
		want           string
	}{
		{
			name: "no signal at all defaults to lt",
			want: "lt",
		},
		{
			name:      "explicit override to en",
			langParam: "en",
			want:      "en",
		},
		{
			name:           "explicit override wins over Accept-Language",
			langParam:      "en",
			acceptLanguage: "lt",
			want:           "en",
		},
		{
			name:           "negotiated from Accept-Language when no override",
			acceptLanguage: "en",
			want:           "en",
		},
		{
			name:           "unsupported Accept-Language falls back to lt",
			acceptLanguage: "fr",
			want:           "lt",
		},
		{
			name:      "garbage override falls through to Accept-Language",
			langParam: "not-a-real-tag!!",
			// language.Parse rejects this, so it's as if no override was
			// given -- Accept-Language decides instead of erroring.
			acceptLanguage: "en",
			want:           "en",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/recommendations?lat=0&lon=0&radius_km=1", nil)
			if tc.langParam != "" {
				q := req.URL.Query()
				q.Set("lang", tc.langParam)
				req.URL.RawQuery = q.Encode()
			}
			if tc.acceptLanguage != "" {
				req.Header.Set("Accept-Language", tc.acceptLanguage)
			}

			if got := resolveLanguage(req); got != tc.want {
				t.Errorf("resolveLanguage() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGetRecommendationsResponseBody(t *testing.T) {
	t.Parallel()

	want := []birding.Recommendation{
		{
			HotspotID:   1,
			HotspotName: "Nemunas Delta",
			TopSpecies: []birding.ExpectedSpecies{
				{SpeciesID: 10, CommonName: "Baltasis gandras", Frequency: 0.8},
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/recommendations?lat=55.3167&lon=21.3667&radius_km=25", nil)
	rec := httptest.NewRecorder()

	handleGetRecommendations(testLogger(), fakeRecommendationService{results: want}).ServeHTTP(rec, req)

	if got, wantStatus := rec.Code, http.StatusOK; got != wantStatus {
		t.Fatalf("status = %d, want %d (body: %q)", got, wantStatus, rec.Body.String())
	}

	var got []birding.Recommendation
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body is not valid JSON: %v (body: %q)", err, rec.Body.String())
	}
	if len(got) != 1 || got[0].HotspotName != "Nemunas Delta" {
		t.Errorf("body = %+v, want %+v", got, want)
	}
}
