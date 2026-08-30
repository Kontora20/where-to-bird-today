// Package store owns everything that talks to PostgreSQL: the connection pool
// and, from Phase 1 onwards, the sqlc-generated queries that run against it.
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool limits. Every Postgres connection costs a backend process on the server,
// so these bound a shared resource rather than a local one. Ten is a starting
// point for a single container, not a measured number — revisit it under load.
const (
	maxConnections    = 10
	maxConnectionAge  = time.Hour
	maxConnectionIdle = 5 * time.Minute
	connectionTimeout = 5 * time.Second
	healthCheckPeriod = 30 * time.Second
)

// NewPool opens a connection pool and proves it can reach the database.
//
// The returned pool is safe for concurrent use by many goroutines: create one
// at startup, share it, and close it once. Callers own the Close.
func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing DATABASE_URL: %w", err)
	}

	poolConfig.MaxConns = maxConnections
	poolConfig.MaxConnLifetime = maxConnectionAge
	poolConfig.MaxConnIdleTime = maxConnectionIdle
	poolConfig.HealthCheckPeriod = healthCheckPeriod

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("creating connection pool: %w", err)
	}

	// NewWithConfig does not connect — the pool opens connections lazily, on
	// first use. Pinging here turns a wrong host, database or password into a
	// startup failure with a clear message, instead of a confusing error on
	// whichever request happens to arrive first.
	pingCtx, cancel := context.WithTimeout(ctx, connectionTimeout)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	return pool, nil
}
