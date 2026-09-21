// Package lastseen rebuilds the table of the most recent record of each species
// at each hotspot. Like seasonality it has no external source, only
// observations already in Postgres.
package lastseen

import (
	"context"
	"fmt"

	"github.com/Kontora20/where-to-bird-today/internal/store"
)

// Source is the ingestion_runs.source value this job writes.
const Source = "last-seen"

// Recompute rebuilds hotspot_species_last_seen from observations. It returns
// the number of rows written.
func Recompute(ctx context.Context, queries *store.Queries) (int, error) {
	if err := queries.TruncateLastSeen(ctx); err != nil {
		return 0, fmt.Errorf("clearing last-seen rows: %w", err)
	}
	rows, err := queries.RecomputeLastSeen(ctx)
	if err != nil {
		return 0, fmt.Errorf("recomputing hotspot_species_last_seen: %w", err)
	}
	return int(rows), nil
}
