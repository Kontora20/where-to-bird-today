package birding

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/Kontora20/where-to-bird-today/internal/store"
)

func TestSearchSpeciesMapsRowsAndPassesLanguagePriority(t *testing.T) {
	t.Parallel()

	var gotArgs store.SearchSpeciesParams
	fake := fakeSearchStore{
		searchArgs: &gotArgs,
		searchRows: []store.SearchSpeciesRow{
			{SpeciesID: 10, CommonName: "Baltasis gandras", ScientificName: "Ciconia ciconia"},
		},
	}

	got, err := newSpeciesSearcher(fake).SearchSpecies(context.Background(), "gandr", "lt")
	if err != nil {
		t.Fatalf("SearchSpecies() error = %v", err)
	}

	want := []SpeciesMatch{{SpeciesID: 10, CommonName: "Baltasis gandras", ScientificName: "Ciconia ciconia"}}
	if !slices.Equal(got, want) {
		t.Errorf("SearchSpecies() = %+v, want %+v", got, want)
	}
	if gotArgs.Query != "gandr" || !slices.Equal(gotArgs.LangPriority, []string{"lt", "en"}) || gotArgs.ResultLimit != speciesSearchLimit {
		t.Errorf("store args = %+v, want query %q, lang priority [lt en], limit %d", gotArgs, "gandr", speciesSearchLimit)
	}
}

func TestSearchSpeciesReturnsEmptySliceNotNil(t *testing.T) {
	t.Parallel()

	got, err := newSpeciesSearcher(fakeSearchStore{}).SearchSpecies(context.Background(), "zzz", "en")
	if err != nil {
		t.Fatalf("SearchSpecies() error = %v", err)
	}
	if got == nil {
		t.Fatal("SearchSpecies() = nil, want a non-nil empty slice (nil marshals to JSON null)")
	}
}

func TestSearchSpeciesPropagatesStoreError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("db down")
	_, err := newSpeciesSearcher(fakeSearchStore{searchErr: wantErr}).SearchSpecies(context.Background(), "x", "en")
	if !errors.Is(err, wantErr) {
		t.Errorf("SearchSpecies() error = %v, want wrapping %v", err, wantErr)
	}
}
