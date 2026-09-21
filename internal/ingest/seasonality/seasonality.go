// Package seasonality rebuilds species_seasonality, the precomputed
// per-hotspot, per-species, per-week baseline that "what to expect" reads
// from. Unlike the loaders under internal/ingest, it has no external source:
// it derives the whole table from observations already in Postgres.
package seasonality

import (
	"context"
	"fmt"

	"github.com/Kontora20/where-to-bird-today/internal/store"
)

// Source is the ingestion_runs.source value this job writes.
const Source = "species-seasonality"

// priorChecklists is how many checklists the country's frequency counts as
// when a hotspot's frequency is adjusted. The median hotspot-week has 2
// complete checklists and the largest has 41, so 5 outweighs a week with one
// or two checklists and barely moves one with twenty. Raise it to trust
// small samples less.
const priorChecklists = 5

// Recompute clears species_seasonality and rebuilds it from observations. It
// returns the number of rows written.
func Recompute(ctx context.Context, queries *store.Queries) (int, error) {
	if err := queries.TruncateSpeciesSeasonality(ctx); err != nil {
		return 0, fmt.Errorf("clearing species_seasonality: %w", err)
	}

	rows, err := queries.RecomputeSpeciesSeasonality(ctx, priorChecklists)
	if err != nil {
		return 0, fmt.Errorf("recomputing species_seasonality: %w", err)
	}
	return int(rows), nil
}
