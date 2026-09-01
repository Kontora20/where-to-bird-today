// Package ingest holds the bookkeeping shared by every loader under
// internal/ingest/*: run inside one transaction, record the outcome in
// ingestion_runs either way.
package ingest

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Kontora20/where-to-bird-today/internal/store"
)

// LoadFunc performs one ingestion using tx-scoped queries and returns the
// row count to record on success.
type LoadFunc func(ctx context.Context, queries *store.Queries) (rowCount int, err error)

// Run records a "running" ingestion_runs row, calls fn inside a single
// transaction, and records the result: "succeeded" with fn's row count if fn
// and the commit both succeed, "failed" with fn's error otherwise. fn's
// writes only ever land if fn returns no error and the commit succeeds.
func Run(ctx context.Context, pool *pgxpool.Pool, queries *store.Queries, source string, fn LoadFunc) (int, error) {
	ingestionRun, err := queries.CreateIngestionRun(ctx, source)
	if err != nil {
		return 0, fmt.Errorf("recording ingestion run: %w", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, finish(ctx, queries, ingestionRun.ID, pgtype.Int4{}, err)
	}

	count, loadErr := fn(ctx, queries.WithTx(tx))
	if loadErr != nil {
		_ = tx.Rollback(ctx)
		return 0, finish(ctx, queries, ingestionRun.ID, pgtype.Int4{}, loadErr)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, finish(ctx, queries, ingestionRun.ID, pgtype.Int4{}, fmt.Errorf("committing: %w", err))
	}

	rowCount := pgtype.Int4{Int32: int32(count), Valid: true}
	if err := finish(ctx, queries, ingestionRun.ID, rowCount, nil); err != nil {
		return 0, err
	}
	return count, nil
}

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
