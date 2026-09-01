// Command ingest-ebird-hotspots fetches every eBird hotspot in cfg.Regions
// and loads it into the hotspots table.
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
		slog.Error("ingest-ebird-hotspots failed", slog.Any("error", err))
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

	count, err := ingest.Run(ctx, pool, queries, ebird.HotspotSource, func(ctx context.Context, q *store.Queries) (int, error) {
		return ebird.LoadHotspots(ctx, client, q, cfg.Regions)
	})
	if err != nil {
		return err
	}

	slog.Info("ebird hotspots ingest complete", slog.Int("hotspots", count), slog.Any("regions", cfg.Regions))
	return nil
}
