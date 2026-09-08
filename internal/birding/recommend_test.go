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
	names        []store.GetPrimaryCommonNamesRow
}

func (f fakeStore) RecommendHotspots(_ context.Context, _ store.RecommendHotspotsParams) ([]store.RecommendHotspotsRow, error) {
	if f.recommendErr != nil {
		return nil, f.recommendErr
	}
	return f.rows, nil
}

func (f fakeStore) GetPrimaryCommonNames(_ context.Context, _ []int64) ([]store.GetPrimaryCommonNamesRow, error) {
	return f.names, nil
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
		names: []store.GetPrimaryCommonNamesRow{
			{SpeciesID: 10, Lang: "lt", CommonName: "Baltasis gandras"},
			{SpeciesID: 10, Lang: "en", CommonName: "White Stork"},
			{SpeciesID: 11, Lang: "en", CommonName: "Mallard"},
		},
	}

	got, err := NewService(fake).Recommend(context.Background(), Params{Latitude: 55.3, Longitude: 21.3, RadiusKM: 25, Week: 20})
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
		t.Errorf("TopSpecies[0].CommonName = %q, want %q (lt should win over en)", got, want)
	}
	if got, want := first.TopSpecies[1].CommonName, "Mallard"; got != want {
		t.Errorf("TopSpecies[1].CommonName = %q, want %q (falls back to en, no lt row)", got, want)
	}

	second := got[1]
	if second.HotspotID != 2 || len(second.TopSpecies) != 0 {
		t.Errorf("got[1] = %+v, want HotspotID=2 with no species", second)
	}
}

func TestRecommendFallsBackToScientificName(t *testing.T) {
	t.Parallel()

	fake := fakeStore{
		rows: []store.RecommendHotspotsRow{
			{HotspotID: 1, HotspotName: "Nemunas Delta", Latitude: pgtypeFloat8(55.3), Longitude: pgtypeFloat8(21.3), DistanceMeters: pgtypeFloat8(5000), Score: 1, SpeciesID: pgtypeInt8(99), ScientificName: pgtypeText("Rallus aquaticus"), Frequency: pgtypeFloat4(0.4)},
		},
		// No rows in `names`: species 99 has no common name in any language.
	}

	got, err := NewService(fake).Recommend(context.Background(), Params{Latitude: 55.3, Longitude: 21.3, RadiusKM: 25, Week: 20})
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

func TestRecommendPropagatesStoreError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("db down")
	_, err := NewService(fakeStore{recommendErr: wantErr}).Recommend(context.Background(), Params{})
	if !errors.Is(err, wantErr) {
		t.Errorf("Recommend() error = %v, want wrapping %v", err, wantErr)
	}
}
