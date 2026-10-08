package api

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// pingTimeout bounds the database round trip a readiness check makes. A probe
// that hangs is worse than one that fails: the orchestrator learns nothing
// while it waits, and its own probe timeout decides the outcome instead.
const pingTimeout = 2 * time.Second

// readyCacheFor is how long a readiness answer is reused. The endpoint is on
// the public address, and each real check takes a database connection.
const readyCacheFor = 5 * time.Second

// pinger is anything that can confirm the database is reachable. It is
// declared here rather than in store so that the handler depends on the one
// method it uses, and so that tests can supply their own.
type pinger interface {
	Ping(ctx context.Context) error
}

// readyResponse is the body of GET /ready, in both the healthy and the
// unhealthy case.
type readyResponse struct {
	Status   string `json:"status"`
	Database string `json:"database"`
}

// handleReady reports whether this instance can serve real traffic.
//
// Unlike /health, this one does check the database. The distinction is what the
// caller should do about a failure: a failing liveness probe means "restart
// this container", a failing readiness probe means "stop sending it requests
// until it recovers". Restarting a process because Postgres blipped fixes
// nothing and loses the connections it had.
func handleReady(logger *slog.Logger, db pinger) http.Handler {
	var (
		mu        sync.Mutex
		checkedAt time.Time
		pingErr   error
	)
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// The answer is shared by every caller for readyCacheFor, so the ping
		// is not tied to the context of whichever request happened to start it.
		mu.Lock()
		if checkedAt.IsZero() || time.Since(checkedAt) >= readyCacheFor {
			ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
			pingErr = db.Ping(ctx)
			cancel()
			checkedAt = time.Now()
		}
		err := pingErr
		mu.Unlock()

		if err != nil {
			logger.Warn("readiness check failed", slog.Any("error", err))
			writeJSON(w, logger, http.StatusServiceUnavailable, readyResponse{
				Status:   "unavailable",
				Database: "unreachable",
			})
			return
		}

		writeJSON(w, logger, http.StatusOK, readyResponse{
			Status:   "ready",
			Database: "reachable",
		})
	})
}
