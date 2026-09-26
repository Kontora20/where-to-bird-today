package api

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// searchRequestsPerMinute bounds each client address to this many calls to
// the search endpoints (recommendations, area weeks, in-season) per minute.
// Each one costs a PostGIS query and, with a drive-time limit, an OSRM table
// request, so an address stepping its coordinates to dodge routing.Cache
// still has a limit (master plan row 31).
const searchRequestsPerMinute = 60

// searchRateLimiter is a per-address fixed-window counter: searchRequestsPerMinute
// tokens, refilled once a minute. No external dependency and no persistence --
// a restart resets everyone's window, which is fine for a limiter whose job is
// only to stop a burst from one address.
type searchRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	now     func() time.Time // overridden in tests
}

type bucket struct {
	remaining int
	resetAt   time.Time
}

func newSearchRateLimiter() *searchRateLimiter {
	return &searchRateLimiter{buckets: make(map[string]*bucket), now: time.Now}
}

// allow reports whether address has a token left in its current window,
// spending one if so.
//
// ponytail: buckets is never swept, so an address seen once is never
// forgotten; add an eviction pass if memory becomes an issue at real traffic.
func (l *searchRateLimiter) allow(address string) bool {
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[address]
	if !ok || !now.Before(b.resetAt) {
		b = &bucket{remaining: searchRequestsPerMinute, resetAt: now.Add(time.Minute)}
		l.buckets[address] = b
	}
	if b.remaining <= 0 {
		return false
	}
	b.remaining--
	return true
}

// rateLimitSearches answers 429 once address has spent its window, otherwise
// passes the request through unchanged.
func rateLimitSearches(limiter *searchRateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !limiter.allow(clientAddress(r)) {
			http.Error(w, "too many searches; slow down", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientAddress reads the address Azure Container Apps' ingress sets in
// X-Forwarded-For, falling back to the direct connection for local
// development where nothing sets that header. A comma-separated list means
// proxies before ours added their own hop; the first entry is the client.
func clientAddress(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		address, _, _ := strings.Cut(forwarded, ",")
		return strings.TrimSpace(address)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
