package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

// testLogger returns a logger that discards output, so tests do not spew JSON.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestRouterStatusCodes is a table-driven test. The cases are ordinary data in
// an ordinary slice, so building them can use loops, helpers, or anything else
// the language can do.
//
// Two of these cases assert behaviour nobody wrote code for: registering only
// "GET /health" makes the Go 1.22+ ServeMux answer 405 for other methods on
// that path and 404 for unknown paths, automatically.
func TestRouterStatusCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		target     string
		wantStatus int
	}{
		{
			name:       "GET /health succeeds",
			method:     http.MethodGet,
			target:     "/health",
			wantStatus: http.StatusOK,
		},
		{
			name:       "POST /health is method not allowed",
			method:     http.MethodPost,
			target:     "/health",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "unknown path is not found",
			method:     http.MethodGet,
			target:     "/does-not-exist",
			wantStatus: http.StatusNotFound,
		},
	}

	router := NewRouter(testLogger(), fakeDatabase{}, fakeIngestionRunStore{}, fakeHotspotStore{}, fakeRecommendationService{})

	for _, tc := range tests {
		// Since Go 1.22 each loop iteration gets its own `tc`, so subtests
		// running in parallel see the right case. Before 1.22 this needed
		// `tc := tc` — one of Go's most notorious bugs, now fixed in the
		// language rather than papered over with a lint rule.
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(tc.method, tc.target, nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("%s %s status = %d, want %d", tc.method, tc.target, rec.Code, tc.wantStatus)
			}
		})
	}
}

// TestHealthResponseBody checks the contract the endpoint actually promises:
// a JSON content type and a body of {"status":"ok"}.
//
// httptest.NewRecorder is an in-memory ResponseWriter, so this exercises the
// real router and the real handler with no socket and no port.
func TestHealthResponseBody(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	NewRouter(testLogger(), fakeDatabase{}, fakeIngestionRunStore{}, fakeHotspotStore{}, fakeRecommendationService{}).ServeHTTP(rec, req)

	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("GET /health status = %d, want %d", got, want)
	}

	if got, want := rec.Header().Get("Content-Type"), "application/json"; got != want {
		t.Errorf("GET /health Content-Type = %q, want %q", got, want)
	}

	var body healthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("GET /health body is not valid JSON: %v (body: %q)", err, rec.Body.String())
	}

	if got, want := body.Status, "ok"; got != want {
		t.Errorf("GET /health body.status = %q, want %q", got, want)
	}
}
