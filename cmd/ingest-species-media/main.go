// Command ingest-species-media resolves photos for every species actually
// observed in the Baltics, via Wikidata/Commons first, then iNaturalist to
// fill gaps -- the provider chain ADR 0003 decided. Both APIs are keyless.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/Kontora20/where-to-bird-today/internal/config"
	"github.com/Kontora20/where-to-bird-today/internal/ingest"
	"github.com/Kontora20/where-to-bird-today/internal/ingest/inaturalist"
	"github.com/Kontora20/where-to-bird-today/internal/ingest/media"
	"github.com/Kontora20/where-to-bird-today/internal/ingest/wikidata"
	"github.com/Kontora20/where-to-bird-today/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("ingest-species-media failed", slog.Any("error", err))
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

	species, err := queries.ListRelevantSpecies(ctx)
	if err != nil {
		return fmt.Errorf("listing relevant species: %w", err)
	}

	providers := []media.Provider{
		wikidata.NewClient(nil),
		inaturalist.NewClient(nil),
	}

	var stats media.Stats
	_, err = ingest.Run(ctx, pool, queries, media.Source, func(ctx context.Context, q *store.Queries) (int, error) {
		var syncErr error
		stats, syncErr = media.Sync(ctx, q, species, providers)

		total := 0
		for _, p := range stats.Providers {
			total += p.PhotosWritten
		}
		return total, syncErr
	})
	if err != nil {
		return err
	}

	for _, p := range stats.Providers {
		slog.Info("species-media provider complete",
			slog.String("provider", p.Provider),
			slog.Int("species_found", p.SpeciesFound),
			slog.Int("photos_written", p.PhotosWritten))
	}
	slog.Info("species-media ingest complete", slog.Int("species_considered", len(species)))
	return nil
}
