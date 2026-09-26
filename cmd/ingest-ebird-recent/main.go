// Command ingest-ebird-recent fetches the latest eBird records of each species
// at each hotspot in cfg.Regions and moves hotspot_species_last_seen forward.
// Meant to run daily.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/Kontora20/where-to-bird-today/internal/config"
	"github.com/Kontora20/where-to-bird-today/internal/ingest"
	"github.com/Kontora20/where-to-bird-today/internal/ingest/ebird"
	"github.com/Kontora20/where-to-bird-today/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("ingest-ebird-recent failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	client, err := ebird.NewClient(cfg.EbirdAPIKey, nil)
	if err != nil {
		return fmt.Errorf("building eBird client: %w", err)
	}

	pool, err := store.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	queries := store.New(pool)

	count, err := ingest.Run(ctx, pool, queries, ebird.RecentSource, func(ctx context.Context, q *store.Queries) (int, error) {
		return ebird.LoadRecent(ctx, client, q, cfg.Regions)
	})
	if err != nil {
		return err
	}

	slog.Info("ebird recent observations ingest complete", slog.Int("last_seen_rows_moved", count), slog.Any("regions", cfg.Regions))
	return nil
}
