package api

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecurityHeaders(t *testing.T) {
	t.Parallel()

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	rec := httptest.NewRecorder()

	securityHeaders(next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	for name, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
		"Permissions-Policy":     "geolocation=(self), camera=(), microphone=()",
	} {
		if got := rec.Header().Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestSearchAnswersAreCacheableButErrorsAreNot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query string
		want  string
	}{
		{name: "found", query: "q=stork", want: "public, max-age=300"},
		{name: "rejected", query: "q=", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/species?"+tc.query, nil)

			handleSearchSpecies(testLogger(), fakeSpeciesSearcher{}).ServeHTTP(rec, req)

			if got := rec.Header().Get("Cache-Control"); got != tc.want {
				t.Errorf("Cache-Control = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRefusedRequestsAreLoggedBelowInfo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     int
		wantLogged bool
	}{
		{name: "served", status: http.StatusOK, wantLogged: true},
		{name: "rate limited", status: http.StatusTooManyRequests},
		{name: "blocked", status: http.StatusForbidden},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var output strings.Builder
			logger := slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelInfo}))
			handler := logRequests(logger, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			}))

			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/species?q=x", nil))

			if got := output.Len() > 0; got != tc.wantLogged {
				t.Errorf("logged = %t, want %t (output: %q)", got, tc.wantLogged, output.String())
			}
		})
	}
}
