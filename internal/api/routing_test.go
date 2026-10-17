package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWarmRoutingAnswersAtOnceAndStartsTheWarmUp(t *testing.T) {
	t.Parallel()

	var warmed int
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/routing/warm", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/130")

	handleWarmRouting(func() { warmed++ }).ServeHTTP(rec, req)

	if got, want := rec.Code, http.StatusNoContent; got != want {
		t.Errorf("status = %d, want %d", got, want)
	}
	if warmed != 1 {
		t.Errorf("warm-up started %d times, want 1", warmed)
	}
}

func TestWarmRoutingWithoutARoutingServerStillAnswers(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()

	handleWarmRouting(nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/routing/warm", nil))

	if got, want := rec.Code, http.StatusNoContent; got != want {
		t.Errorf("status = %d, want %d", got, want)
	}
}
