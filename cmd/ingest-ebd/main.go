// Command ingest-ebd loads one eBird Basic Dataset file into the checklists
// and observations tables. Run it once per file -- eBird's own
// custom-download tool produces one export per requested region, so a
// second region is a second export and a second run, not a second argument.
// An export's sampling event file (the *_sampling.txt beside the main file)
// is loaded the same way and adds the checklists the main file lacks.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/Kontora20/where-to-bird-today/internal/config"
	"github.com/Kontora20/where-to-bird-today/internal/ingest"
	"github.com/Kontora20/where-to-bird-today/internal/ingest/ebd"
	"github.com/Kontora20/where-to-bird-today/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("ingest-ebd failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: ingest-ebd <path-to-ebd-file.txt>")
	}
	path := os.Args[1]

	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening EBD export: %w", err)
	}
	defer func() { _ = file.Close() }()

	pool, err := store.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	queries := store.New(pool)

	var stats ebd.Stats
	_, err = ingest.Run(ctx, pool, queries, ebd.Source, func(ctx context.Context, q *store.Queries) (int, error) {
		var loadErr error
		stats, loadErr = ebd.Load(ctx, q, file)
		return stats.Checklists + stats.Inserted, loadErr
	})
	if err != nil {
		return err
	}

	if stats.SkippedUnmatchedSpecies > 0 {
		slog.Warn("ebd ingest: rows with unmatched species", slog.Int("skipped", stats.SkippedUnmatchedSpecies))
	}
	slog.Info("ebd ingest complete",
		slog.Int("checklists", stats.Checklists),
		slog.Int("observations", stats.Inserted),
		slog.Int("skipped_filtered", stats.SkippedFiltered),
		slog.String("file", path))
	return nil
}
