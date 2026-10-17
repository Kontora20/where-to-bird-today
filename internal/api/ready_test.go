package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
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

			NewRouter(testLogger(), fakeDatabase{err: tc.pingErr}, fakeRecommendationService{}, fstest.MapFS{}, nil).ServeHTTP(rec, req)

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

// countingDatabase counts its pings.
type countingDatabase struct {
	pings *int
}

func (c countingDatabase) Ping(_ context.Context) error {
	*c.pings++
	return nil
}

func TestReadyReusesTheAnswerForAFewSeconds(t *testing.T) {
	t.Parallel()

	pings := 0
	handler := handleReady(testLogger(), countingDatabase{pings: &pings})

	for range 5 {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ready", nil))
	}

	if pings != 1 {
		t.Errorf("database pinged %d times by 5 calls in a row, want 1", pings)
	}
}
