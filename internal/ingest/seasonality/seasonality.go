// Package seasonality rebuilds species_seasonality, the precomputed
// per-hotspot, per-species, per-week baseline that "what to expect" reads
// from, and country_seasonality, the same per country. Unlike the loaders under internal/ingest, it has no external source:
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

// Recompute clears species_seasonality and country_seasonality and rebuilds
// them from checklists and observations, the country table from the hotspot
// one. It returns the number of rows written to both.
func Recompute(ctx context.Context, queries *store.Queries) (int, error) {
	if err := queries.TruncateSeasonality(ctx); err != nil {
		return 0, fmt.Errorf("clearing seasonality: %w", err)
	}

	hotspotRows, err := queries.RecomputeSpeciesSeasonality(ctx, priorChecklists)
	if err != nil {
		return 0, fmt.Errorf("recomputing species_seasonality: %w", err)
	}
	countryRows, err := queries.RecomputeCountrySeasonality(ctx)
	if err != nil {
		return 0, fmt.Errorf("recomputing country_seasonality: %w", err)
	}
	return int(hotspotRows + countryRows), nil
}
