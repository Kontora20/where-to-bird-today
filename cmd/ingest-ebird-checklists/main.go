// Command ingest-ebird-checklists loads the checklists made at known hotspots
// in cfg.Regions since the newest one stored, from the eBird API, so the
// seasonality tables can be rebuilt from current data without waiting for the
// next EBD export. Meant to run daily, before compute-seasonality.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/Kontora20/where-to-bird-today/internal/config"
	"github.com/Kontora20/where-to-bird-today/internal/ingest"
	"github.com/Kontora20/where-to-bird-today/internal/ingest/ebird"
	"github.com/Kontora20/where-to-bird-today/internal/store"
)

// overlapDays is how far before the newest stored checklist a run starts
// again, because people submit checklists days after the birding.
const overlapDays = 3

// firstRunDays is how far back the very first run looks.
const firstRunDays = 30

// timeBudget stops a run between days, so a long catch-up ends well inside
// the job's timeout. The next run carries on from where this one stopped, as
// it does when the hourly request limit is reached.
const timeBudget = 50 * time.Minute

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("ingest-ebird-checklists failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()
	started := time.Now()

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

	today := started.UTC().Truncate(24 * time.Hour)
	first, err := firstDay(ctx, queries, cfg.Regions, today)
	if err != nil {
		return err
	}

	total := 0
	for day := first; !day.After(today); day = day.AddDate(0, 0, 1) {
		if time.Since(started) > timeBudget {
			slog.Warn("time budget used, the next run carries on", slog.String("stopped_before", day.Format(time.DateOnly)))
			break
		}

		count, err := ingest.Run(ctx, pool, queries, ebird.ChecklistsSource, func(ctx context.Context, q *store.Queries) (int, error) {
			return ebird.LoadChecklists(ctx, client, q, cfg.Regions, day)
		})
		if errors.Is(err, ebird.ErrRateLimited) {
			slog.Warn("eBird hourly limit reached, the next run carries on", slog.String("stopped_during", day.Format(time.DateOnly)))
			break
		}
		if err != nil {
			return fmt.Errorf("loading %s: %w", day.Format(time.DateOnly), err)
		}
		total += count
	}

	slog.Info("ebird checklists ingest complete", slog.Int("checklists_stored", total), slog.Any("regions", cfg.Regions), slog.Duration("took", time.Since(started)))
	return nil
}

// firstDay is the day a run starts from: overlapDays before the newest
// checklist stored for the region that is furthest behind, so one country that
// fell behind is caught up even when the others are current.
func firstDay(ctx context.Context, queries *store.Queries, regions []string, today time.Time) (time.Time, error) {
	first := today
	for _, region := range regions {
		start := today.AddDate(0, 0, -firstRunDays)
		latest, err := queries.GetLatestChecklistDate(ctx, region)
		if err != nil {
			return time.Time{}, fmt.Errorf("reading the newest %s checklist date: %w", region, err)
		}
		if latest.Valid {
			start = latest.Time.AddDate(0, 0, -overlapDays)
		}
		if start.Before(first) {
			first = start
		}
	}
	return first, nil
}
