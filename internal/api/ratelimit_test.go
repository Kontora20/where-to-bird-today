package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSearchRateLimiterAllow(t *testing.T) {
	t.Parallel()

	now := time.Now()
	limiter := newSearchRateLimiter(searchRequestsPerMinute)
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

	limiter := newSearchRateLimiter(searchRequestsPerMinute)
	limiter.now = time.Now
	handler := rateLimitSearches(limiter, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
		{name: "several hops takes the last", forwarded: "6.6.6.6, 1.2.3.4", remoteAddr: "10.0.0.2:5678", want: "1.2.3.4"},
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

func TestSearchRateLimiterDropsEndedWindows(t *testing.T) {
	t.Parallel()

	now := time.Now()
	limiter := newSearchRateLimiter(searchRequestsPerMinute)
	limiter.now = func() time.Time { return now }

	for i := range maxTrackedAddresses {
		limiter.allow(fmt.Sprintf("10.0.%d.%d", i/256, i%256))
	}
	now = now.Add(time.Minute)
	limiter.allow("1.2.3.4")

	if got := len(limiter.buckets); got != 1 {
		t.Errorf("tracked addresses after the windows ended = %d, want 1", got)
	}
}

func TestRateLimitSearchesSaysWhenToRetry(t *testing.T) {
	t.Parallel()

	limiter := newSearchRateLimiter(1)
	handler := rateLimitSearches(limiter, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/recommendations", nil)
	req.RemoteAddr = "9.9.9.9:12345"

	handler.ServeHTTP(httptest.NewRecorder(), req)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got, want := rec.Header().Get("Retry-After"), "60"; got != want {
		t.Errorf("Retry-After = %q, want %q", got, want)
	}
}

func TestRateLimitDriveTimeSearchesOnlyCountsDriveTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		query      string
		wantStatus int
	}{
		{name: "drive time is limited", query: "?lat=54.7&lon=25.3&max_minutes=60", wantStatus: http.StatusTooManyRequests},
		{name: "radius is not", query: "?lat=54.7&lon=25.3&radius_km=50", wantStatus: http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handler := rateLimitDriveTimeSearches(newSearchRateLimiter(1), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
			req := httptest.NewRequest(http.MethodGet, "/recommendations"+tc.query, nil)
			req.RemoteAddr = "9.9.9.9:12345"

			handler.ServeHTTP(httptest.NewRecorder(), req)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("second request: status = %d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}
