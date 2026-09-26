package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSearchRateLimiterAllow(t *testing.T) {
	t.Parallel()

	now := time.Now()
	limiter := newSearchRateLimiter()
	limiter.now = func() time.Time { return now }

	for i := range searchRequestsPerMinute {
		if !limiter.allow("1.2.3.4") {
			t.Fatalf("allow() call %d = false, want true (still inside the limit)", i+1)
		}
	}
	if limiter.allow("1.2.3.4") {
		t.Error("allow() call after the limit = true, want false")
	}

	// A different address has its own budget.
	if !limiter.allow("5.6.7.8") {
		t.Error("allow() for a different address = false, want true")
	}

	// Once the window has passed, the address is allowed again.
	now = now.Add(time.Minute)
	if !limiter.allow("1.2.3.4") {
		t.Error("allow() after the window reset = false, want true")
	}
}

func TestRateLimitSearchesAnswers429OnceSpent(t *testing.T) {
	t.Parallel()

	limiter := newSearchRateLimiter()
	limiter.now = time.Now
	handler := rateLimitSearches(limiter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/recommendations", nil)
	req.RemoteAddr = "9.9.9.9:12345"

	for i := range searchRequestsPerMinute {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d status = %d, want %d", i+1, rec.Code, http.StatusOK)
		}
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("request over the limit: status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
}

func TestClientAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		forwarded  string
		remoteAddr string
		want       string
	}{
		{name: "no proxy", remoteAddr: "1.2.3.4:5678", want: "1.2.3.4"},
		{name: "one hop", forwarded: "1.2.3.4", remoteAddr: "10.0.0.1:5678", want: "1.2.3.4"},
		{name: "several hops takes the first", forwarded: "1.2.3.4, 10.0.0.1", remoteAddr: "10.0.0.2:5678", want: "1.2.3.4"},
		{name: "remote addr with no port", remoteAddr: "1.2.3.4", want: "1.2.3.4"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/recommendations", nil)
			req.RemoteAddr = tc.remoteAddr
			if tc.forwarded != "" {
				req.Header.Set("X-Forwarded-For", tc.forwarded)
			}

			if got := clientAddress(req); got != tc.want {
				t.Errorf("clientAddress() = %q, want %q", got, tc.want)
			}
		})
	}
}
