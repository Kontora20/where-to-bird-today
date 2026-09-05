// Command compute-seasonality rebuilds species_seasonality from the
// observations currently in Postgres. Safe to re-run any time observations
// changes -- it always recomputes the whole table, never incrementally.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/Kontora20/where-to-bird-today/internal/config"
	"github.com/Kontora20/where-to-bird-today/internal/ingest"
	"github.com/Kontora20/where-to-bird-today/internal/ingest/seasonality"
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
	return nil
}
