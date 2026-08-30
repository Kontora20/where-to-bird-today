// Package api contains the application's HTTP surface: routing, middleware,
// and handlers.
package api

import (
	"log/slog"
	"net/http"
	"time"
)

// NewRouter builds the application's complete HTTP handler.
//
// It returns an http.Handler rather than a *http.ServeMux so that middleware
// can wrap it — the caller only needs "something that can serve requests", and
// should not be able to reach in and register more routes after the fact.
//
// Dependencies arrive as parameters and are handed to the handlers that need
// them. The database is taken as a pinger, not as a concrete pool: the router
// passes on what it is given and never touches the database itself.
func NewRouter(logger *slog.Logger, db pinger, ingestionRuns ingestionRunStore) http.Handler {
	mux := http.NewServeMux()

	// "GET /health" is a Go 1.22+ pattern: method and path in one string.
	// Registering only GET means the mux answers 405 (with an Allow header)
	// for other methods on this path, and 404 for unknown paths — both for
	// free, with no configuration.
	mux.Handle("GET /health", handleHealth(logger))
	mux.Handle("GET /ready", handleReady(logger, db))
	mux.Handle("POST /ingestion-runs", handleCreateIngestionRun(logger, ingestionRuns))
	mux.Handle("GET /ingestion-runs/{id}", handleGetIngestionRun(logger, ingestionRuns))

	// Middleware is applied by wrapping. The outermost wrapper runs first.
	return logRequests(logger, mux)
}

// logRequests logs one structured line per request, after it completes.
//
// This is the whole middleware concept in Go: a function taking an
// http.Handler and returning a new one that does something extra around it.
// There is no registration, no pipeline object, and no ordering configuration
// — the order is simply the order you nest the calls.
func logRequests(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// The real ResponseWriter will not tell us what status was written,
		// so we substitute one that remembers.
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		logger.Info("http request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rec.status),
			slog.Duration("duration", time.Since(start)),
		)
	})
}

// statusRecorder wraps an http.ResponseWriter to capture the status code.
//
// The embedded http.ResponseWriter (a field with no name) promotes all of its
// methods onto statusRecorder, so this type satisfies http.ResponseWriter
// while only actually implementing one method itself. WriteHeader below
// shadows the promoted method and calls through to it.
//
// Caveat worth knowing: wrapping a ResponseWriter hides any optional
// interfaces the original also implemented (http.Flusher, http.Hijacker,
// io.ReaderFrom). That does not matter for JSON responses, but it will if we
// ever add streaming or Server-Sent Events.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
