package birding

import (
	"context"
	"fmt"

	"github.com/Kontora20/where-to-bird-today/internal/store"
)

// speciesSearchLimit caps the suggestions the target-species picker shows.
const speciesSearchLimit = 10

// SpeciesMatch is one suggestion for the target-species picker.
type SpeciesMatch struct {
	SpeciesID      int64  `json:"species_id"`
	CommonName     string `json:"common_name"`
	ScientificName string `json:"scientific_name"`
}

// searchStore is what SpeciesSearcher needs from persistence.
type searchStore interface {
	SearchSpecies(ctx context.Context, arg store.SearchSpeciesParams) ([]store.SearchSpeciesRow, error)
}

// SpeciesSearcher finds species by name for the target-species picker.
type SpeciesSearcher struct {
	store searchStore
}

func newSpeciesSearcher(store searchStore) *SpeciesSearcher {
	return &SpeciesSearcher{store: store}
}

// SearchSpecies returns species whose name contains query, named in lang
// (falling back to the other supported language, then the scientific name).
func (s *SpeciesSearcher) SearchSpecies(ctx context.Context, query, lang string) ([]SpeciesMatch, error) {
	rows, err := s.store.SearchSpecies(ctx, store.SearchSpeciesParams{
		Query:        query,
		LangPriority: []string{lang, otherOf(lang)},
		ResultLimit:  speciesSearchLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("searching species: %w", err)
	}

	matches := make([]SpeciesMatch, 0, len(rows))
	for _, row := range rows {
		matches = append(matches, SpeciesMatch{
			SpeciesID:      row.SpeciesID,
			CommonName:     row.CommonName,
			ScientificName: row.ScientificName,
		})
	}
	return matches, nil
}
