package ebird

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Kontora20/where-to-bird-today/internal/store"
)

// Source is the ingestion_runs.source value the name loader writes.
const Source = "ebird-taxonomy"

// Locales this project ships. Lithuanian first: it is the default whenever
// a request doesn't say otherwise.
var Locales = []string{"lt", "en"}

// LoadNames fetches the eBird taxonomy for each of Locales and upserts every
// species-rank entry into species_names. It returns the number of rows
// written and the number of taxonomy entries whose species code had no
// matching row in species -- expected to be small, not zero, since AviList
// and eBird revise their taxonomies independently.
func LoadNames(ctx context.Context, client *Client, queries *store.Queries) (written, unmatched int, err error) {
	for _, locale := range Locales {
		entries, err := client.Taxonomy(ctx, locale)
		if err != nil {
			return written, unmatched, fmt.Errorf("fetching %s taxonomy: %w", locale, err)
		}

		for _, entry := range entries {
			rows, err := queries.UpsertSpeciesName(ctx, store.UpsertSpeciesNameParams{
				EbirdSpeciesCode: pgtype.Text{String: entry.SpeciesCode, Valid: true},
				Lang:             locale,
				CommonName:       entry.CommonName,
				Source:           "ebird",
				IsPrimary:        true,
			})
			if err != nil {
				return written, unmatched, fmt.Errorf("upserting %s name for %s: %w", locale, entry.SpeciesCode, err)
			}
			if rows == 0 {
				unmatched++
				continue
			}
			written++
		}
	}

	return written, unmatched, nil
}
