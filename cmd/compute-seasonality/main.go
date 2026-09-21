// Command compute-seasonality rebuilds the tables derived from the checklists
// and observations currently in Postgres: species_seasonality, and the
// time-of-day counts the species panel reads. Safe to re-run any time they
// change -- it always recomputes everything, never incrementally.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/Kontora20/where-to-bird-today/internal/config"
	"github.com/Kontora20/where-to-bird-today/internal/ingest"
	"github.com/Kontora20/where-to-bird-today/internal/ingest/seasonality"
	"github.com/Kontora20/where-to-bird-today/internal/ingest/timeofday"
	"github.com/Kontora20/where-to-bird-today/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("compute-seasonality failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	pool, err := store.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	queries := store.New(pool)

	count, err := ingest.Run(ctx, pool, queries, seasonality.Source, seasonality.Recompute)
	if err != nil {
		return err
	}

	slog.Info("species_seasonality recomputed", slog.Int("rows", count))

	count, err = ingest.Run(ctx, pool, queries, timeofday.Source, timeofday.Recompute)
	if err != nil {
		return err
	}

	slog.Info("time-of-day counts recomputed", slog.Int("rows", count))
	return nil
}
