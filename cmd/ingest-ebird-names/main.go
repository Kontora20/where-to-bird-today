// Command ingest-ebird-names fetches the eBird taxonomy in every supported
// locale and loads it into species_names.
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
		slog.Error("ingest-ebird-names failed", slog.Any("error", err))
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

	var unmatched int
	written, err := ingest.Run(ctx, pool, queries, ebird.Source, func(ctx context.Context, q *store.Queries) (int, error) {
		w, u, err := ebird.LoadNames(ctx, client, q)
		unmatched = u
		return w, err
	})
	if err != nil {
		return err
	}

	slog.Info("ebird names ingest complete", slog.Int("written", written), slog.Int("unmatched", unmatched))
	return nil
}
