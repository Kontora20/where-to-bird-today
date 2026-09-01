// Command ingest-avilist loads the embedded AviList species CSV into the
// species table.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Kontora20/where-to-bird-today/internal/config"
	"github.com/Kontora20/where-to-bird-today/internal/ingest/avilist"
	"github.com/Kontora20/where-to-bird-today/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("ingest-avilist failed", slog.Any("error", err))
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

	ingestionRun, err := queries.CreateIngestionRun(ctx, avilist.Source)
	if err != nil {
		return fmt.Errorf("recording ingestion run: %w", err)
	}

	// One transaction: a failed row rolls back everything already upserted.
	tx, err := pool.Begin(ctx)
	if err != nil {
		return finish(ctx, queries, ingestionRun.ID, pgtype.Int4{}, err)
	}

	count, loadErr := avilist.Load(ctx, queries.WithTx(tx))
	if loadErr != nil {
		_ = tx.Rollback(ctx)
		return finish(ctx, queries, ingestionRun.ID, pgtype.Int4{}, loadErr)
	}

	if err := tx.Commit(ctx); err != nil {
		return finish(ctx, queries, ingestionRun.ID, pgtype.Int4{}, fmt.Errorf("committing: %w", err))
	}

	rowCount := pgtype.Int4{Int32: int32(count), Valid: true}
	if err := finish(ctx, queries, ingestionRun.ID, rowCount, nil); err != nil {
		return err
	}

	slog.Info("avilist ingest complete", slog.Int("species", count))
	return nil
}

// finish records the ingestion_runs outcome and returns loadErr unchanged,
// so callers can `return finish(...)` and still propagate the real failure.
func finish(ctx context.Context, queries *store.Queries, runID int64, rowCount pgtype.Int4, loadErr error) error {
	status := "succeeded"
	var errText pgtype.Text
	if loadErr != nil {
		status = "failed"
		errText = pgtype.Text{String: loadErr.Error(), Valid: true}
	}

	if _, err := queries.FinishIngestionRun(ctx, store.FinishIngestionRunParams{
		ID:       runID,
		RowCount: rowCount,
		Status:   status,
		Error:    errText,
	}); err != nil {
		return fmt.Errorf("recording ingestion result: %w", err)
	}

	return loadErr
}
