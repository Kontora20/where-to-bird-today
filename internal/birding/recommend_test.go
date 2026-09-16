package birding

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Kontora20/where-to-bird-today/internal/store"
)

type fakeStore struct {
	recommendErr error
	rows         []store.RecommendHotspotsRow
	// nameCandidates simulates species_names rows: speciesID -> lang -> name.
	nameCandidates map[int64]map[string]string
}

func (f fakeStore) RecommendHotspots(_ context.Context, _ store.RecommendHotspotsParams) ([]store.RecommendHotspotsRow, error) {
	if f.recommendErr != nil {
		return nil, f.recommendErr
	}
	return f.rows, nil
}

// GetCommonNames mimics the real query's DISTINCT ON + array_position
// ranking closely enough to test arg.LangPriority actually mattering: for
// each species, it picks the first language in the priority list that has a
// candidate, skipping species with no candidate in any priority language --
// the same shape as a species with no matching row in real Postgres.
func (f fakeStore) GetCommonNames(_ context.Context, arg store.GetCommonNamesParams) ([]store.GetCommonNamesRow, error) {
	var rows []store.GetCommonNamesRow
	for _, speciesID := range arg.SpeciesIds {
		for _, lang := range arg.LangPriority {
			if name, ok := f.nameCandidates[speciesID][lang]; ok {
				rows = append(rows, store.GetCommonNamesRow{SpeciesID: speciesID, CommonName: name})
				break
			}
		}
	}
	return rows, nil
}

// Named pgtypeX, not X: int8 is a predeclared Go type, so a function named
// int8 would shadow it. The others aren't, but pgtypeX keeps all four
// readable as one family.
func pgtypeFloat8(v float64) pgtype.Float8 { return pgtype.Float8{Float64: v, Valid: true} }
func pgtypeFloat4(v float32) pgtype.Float4 { return pgtype.Float4{Float32: v, Valid: true} }
func pgtypeInt8(v int64) pgtype.Int8       { return pgtype.Int8{Int64: v, Valid: true} }
func pgtypeText(v string) pgtype.Text      { return pgtype.Text{String: v, Valid: true} }

func TestRecommendGroupsRowsByHotspot(t *testing.T) {
	t.Parallel()

	fake := fakeStore{
		rows: []store.RecommendHotspotsRow{
			{HotspotID: 1, HotspotName: "Nemunas Delta", Latitude: pgtypeFloat8(55.3), Longitude: pgtypeFloat8(21.3), DistanceMeters: pgtypeFloat8(5000), Score: 1.5, SpeciesID: pgtypeInt8(10), ScientificName: pgtypeText("Ciconia ciconia"), Frequency: pgtypeFloat4(0.9)},
			{HotspotID: 1, HotspotName: "Nemunas Delta", Latitude: pgtypeFloat8(55.3), Longitude: pgtypeFloat8(21.3), DistanceMeters: pgtypeFloat8(5000), Score: 1.5, SpeciesID: pgtypeInt8(11), ScientificName: pgtypeText("Anas platyrhynchos"), Frequency: pgtypeFloat4(0.6)},
			{HotspotID: 2, HotspotName: "Kaunas Lagoon", Latitude: pgtypeFloat8(54.9), Longitude: pgtypeFloat8(23.9), DistanceMeters: pgtypeFloat8(20000), Score: 0, SpeciesID: pgtype.Int8{}, ScientificName: pgtype.Text{}, Frequency: pgtype.Float4{}},
		},
		nameCandidates: map[int64]map[string]string{
			10: {"lt": "Baltasis gandras", "en": "White Stork"},
			11: {"en": "Mallard"}, // no lt row -- e.g. a rare vagrant, per ADR 0005's noted consequence
		},
	}

	got, err := NewService(fake).Recommend(context.Background(), Params{Latitude: 55.3, Longitude: 21.3, RadiusKM: 25, Week: 20, Lang: "lt"})
	if err != nil {
		t.Fatalf("Recommend() error = %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}

	first := got[0]
	if first.HotspotID != 1 || first.DistanceKM != 5 {
		t.Errorf("got[0] = %+v, want HotspotID=1 DistanceKM=5", first)
	}
	if len(first.TopSpecies) != 2 {
		t.Fatalf("len(got[0].TopSpecies) = %d, want 2", len(first.TopSpecies))
	}
	if got, want := first.TopSpecies[0].CommonName, "Baltasis gandras"; got != want {
		t.Errorf("TopSpecies[0].CommonName = %q, want %q (lt requested and available)", got, want)
	}
	if got, want := first.TopSpecies[1].CommonName, "Mallard"; got != want {
		t.Errorf("TopSpecies[1].CommonName = %q, want %q (falls back to en, no lt row)", got, want)
	}

	second := got[1]
	if second.HotspotID != 2 || len(second.TopSpecies) != 0 {
		t.Errorf("got[1] = %+v, want HotspotID=2 with no species", second)
	}
}

// TestRecommendRespectsRequestedLanguage proves Params.Lang actually
// changes which name comes back, not just that a name comes back --
// the whole point of ADR 0005 making language a per-request parameter.
func TestRecommendRespectsRequestedLanguage(t *testing.T) {
	t.Parallel()

	fake := fakeStore{
		rows: []store.RecommendHotspotsRow{
			{HotspotID: 1, HotspotName: "Nemunas Delta", Latitude: pgtypeFloat8(55.3), Longitude: pgtypeFloat8(21.3), DistanceMeters: pgtypeFloat8(5000), Score: 1, SpeciesID: pgtypeInt8(10), ScientificName: pgtypeText("Ciconia ciconia"), Frequency: pgtypeFloat4(0.9)},
		},
		nameCandidates: map[int64]map[string]string{
			10: {"lt": "Baltasis gandras", "en": "White Stork"},
		},
	}

	got, err := NewService(fake).Recommend(context.Background(), Params{Latitude: 55.3, Longitude: 21.3, RadiusKM: 25, Week: 20, Lang: "en"})
	if err != nil {
		t.Fatalf("Recommend() error = %v", err)
	}

	if len(got) != 1 || len(got[0].TopSpecies) != 1 {
		t.Fatalf("got = %+v, want one hotspot with one species", got)
	}
	if gotName, want := got[0].TopSpecies[0].CommonName, "White Stork"; gotName != want {
		t.Errorf("CommonName = %q, want %q (Lang: \"en\" was requested)", gotName, want)
	}
}

func TestRecommendFallsBackToScientificName(t *testing.T) {
	t.Parallel()

	fake := fakeStore{
		rows: []store.RecommendHotspotsRow{
			{HotspotID: 1, HotspotName: "Nemunas Delta", Latitude: pgtypeFloat8(55.3), Longitude: pgtypeFloat8(21.3), DistanceMeters: pgtypeFloat8(5000), Score: 1, SpeciesID: pgtypeInt8(99), ScientificName: pgtypeText("Rallus aquaticus"), Frequency: pgtypeFloat4(0.4)},
		},
		// No candidates for species 99 in any language.
	}

	got, err := NewService(fake).Recommend(context.Background(), Params{Latitude: 55.3, Longitude: 21.3, RadiusKM: 25, Week: 20, Lang: "lt"})
	if err != nil {
		t.Fatalf("Recommend() error = %v", err)
	}

	if len(got) != 1 || len(got[0].TopSpecies) != 1 {
		t.Fatalf("got = %+v, want one hotspot with one species", got)
	}
	if gotName, want := got[0].TopSpecies[0].CommonName, "Rallus aquaticus"; gotName != want {
		t.Errorf("CommonName = %q, want %q (scientific-name fallback)", gotName, want)
	}
}

// TestRecommendReturnsEmptySliceNotNil guards against a `var recommendations
// []Recommendation` regression: a nil slice marshals to JSON `null`, and the
// frontend's `{#each recommendations}` throws on null instead of rendering
// nothing.
func TestRecommendReturnsEmptySliceNotNil(t *testing.T) {
	t.Parallel()

	got, err := NewService(fakeStore{}).Recommend(context.Background(), Params{Latitude: 55.3, Longitude: 21.3, RadiusKM: 25, Week: 20, Lang: "lt"})
	if err != nil {
		t.Fatalf("Recommend() error = %v", err)
	}

	if got == nil {
		t.Fatal("Recommend() = nil, want a non-nil empty slice")
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0", len(got))
	}
}

func TestRecommendPropagatesStoreError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("db down")
	_, err := NewService(fakeStore{recommendErr: wantErr}).Recommend(context.Background(), Params{})
	if !errors.Is(err, wantErr) {
		t.Errorf("Recommend() error = %v, want wrapping %v", err, wantErr)
	}
}
