package api

import (
	"net"
	"net/http"
	"strconv"
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

// driveTimeRequestsPerMinute is the smaller budget for searches limited by
// drive time, which are the ones that can cost an OSRM request. It is spent
// together with the general one. A person changing the range makes three calls
// each time, so this allows about ten changes a minute.
const driveTimeRequestsPerMinute = 30

// rateLimitWindow is how long a spent budget lasts, and what Retry-After says.
const rateLimitWindow = time.Minute

// maxTrackedAddresses is how many addresses the limiter holds before it drops
// the ones whose window has ended, so many addresses seen once each cannot
// grow its memory without limit.
const maxTrackedAddresses = 10_000

// searchRateLimiter is a per-address fixed-window counter: perMinute tokens,
// refilled once a minute. No external dependency and no persistence --
// a restart resets everyone's window, which is fine for a limiter whose job is
// only to stop a burst from one address.
type searchRateLimiter struct {
	perMinute int
	mu        sync.Mutex
	buckets   map[string]*bucket
	now       func() time.Time // overridden in tests
}

type bucket struct {
	remaining int
	resetAt   time.Time
}

func newSearchRateLimiter(perMinute int) *searchRateLimiter {
	return &searchRateLimiter{perMinute: perMinute, buckets: make(map[string]*bucket), now: time.Now}
}

// allow reports whether address has a token left in its current window,
// spending one if so.
func (l *searchRateLimiter) allow(address string) bool {
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[address]
	if !ok && len(l.buckets) >= maxTrackedAddresses {
		for address, old := range l.buckets {
			if !now.Before(old.resetAt) {
				delete(l.buckets, address)
			}
		}
	}
	if !ok || !now.Before(b.resetAt) {
		b = &bucket{remaining: l.perMinute, resetAt: now.Add(rateLimitWindow)}
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
			w.Header().Set("Retry-After", strconv.Itoa(int(rateLimitWindow.Seconds())))
			http.Error(w, "rate_limited", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// rateLimitDriveTimeSearches applies limiter only to searches limited by
// drive time (?max_minutes=); a search by radius passes through.
func rateLimitDriveTimeSearches(limiter *searchRateLimiter, next http.Handler) http.Handler {
	limited := rateLimitSearches(limiter, next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("max_minutes") == "" {
			next.ServeHTTP(w, r)
			return
		}
		limited.ServeHTTP(w, r)
	})
}

// clientAddress reads the address Azure Container Apps' ingress appends to
// X-Forwarded-For, falling back to the direct connection for local
// development where nothing sets that header. Only the last entry is
// trusted: everything before it was sent by the client, which can write any
// address there to get a fresh rate-limit window on every request.
func clientAddress(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		return strings.TrimSpace(forwarded[strings.LastIndex(forwarded, ",")+1:])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
