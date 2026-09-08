package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeDatabase is a pinger whose answer the test chooses.
type fakeDatabase struct {
	err error
}

func (f fakeDatabase) Ping(_ context.Context) error {
	return f.err
}

func TestReady(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		pingErr      error
		wantStatus   int
		wantDatabase string
	}{
		{
			name:         "database reachable",
			pingErr:      nil,
			wantStatus:   http.StatusOK,
			wantDatabase: "reachable",
		},
		{
			name:         "database unreachable",
			pingErr:      errors.New("connection refused"),
			wantStatus:   http.StatusServiceUnavailable,
			wantDatabase: "unreachable",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/ready", nil)
			rec := httptest.NewRecorder()

			NewRouter(testLogger(), fakeDatabase{err: tc.pingErr}, fakeIngestionRunStore{}, fakeHotspotStore{}, fakeRecommendationService{}).ServeHTTP(rec, req)

			if got := rec.Code; got != tc.wantStatus {
				t.Fatalf("GET /ready status = %d, want %d", got, tc.wantStatus)
			}

			var body readyResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("GET /ready body is not valid JSON: %v (body: %q)", err, rec.Body.String())
			}

			if got := body.Database; got != tc.wantDatabase {
				t.Errorf("GET /ready body.database = %q, want %q", got, tc.wantDatabase)
			}
		})
	}
}
