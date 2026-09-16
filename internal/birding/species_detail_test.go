package birding

import (
	"context"
	"errors"
	"testing"

	"github.com/Kontora20/where-to-bird-today/internal/store"
)

func TestSpeciesDetail(t *testing.T) {
	t.Parallel()

	fake := fakeStore{
		speciesRow: store.Species{
			ID:                  10,
			EbirdSpeciesCode:    pgtypeText("whistor1"),
			ScientificName:      "Ciconia ciconia",
			BirdsOfTheWorldUrl:  pgtypeText("https://birdsoftheworld.org/bow/species/whistor1/"),
			BirdlifeDatazoneUrl: pgtypeText("https://datazone.birdlife.org/species/factsheet/1"),
		},
		nameCandidates: map[int64]map[string]string{
			10: {"lt": "Baltasis gandras", "en": "White Stork"},
		},
		mediaRows: []store.SpeciesMedium{
			{SpeciesID: 10, ImageUrl: "https://example.org/stork-best.jpg", License: "CC BY 4.0", Attribution: "Someone", Rank: 0},
			{SpeciesID: 10, ImageUrl: "https://example.org/stork-second.jpg", License: "CC BY 4.0", Attribution: "Someone Else", Rank: 1},
		},
		seasonalityRows: []store.GetSpeciesSeasonalityAtHotspotRow{
			{WeekOfYear: 18, Frequency: 0.4},
			{WeekOfYear: 19, Frequency: 0.6},
		},
	}

	got, err := NewService(fake).SpeciesDetail(context.Background(), 1, 10, "lt")
	if err != nil {
		t.Fatalf("SpeciesDetail() error = %v", err)
	}

	if got.CommonName != "Baltasis gandras" {
		t.Errorf("CommonName = %q, want %q (lt requested and available)", got.CommonName, "Baltasis gandras")
	}
	if got.ScientificName != "Ciconia ciconia" {
		t.Errorf("ScientificName = %q, want %q", got.ScientificName, "Ciconia ciconia")
	}
	if want := "https://ebird.org/species/whistor1"; got.EBirdURL != want {
		t.Errorf("EBirdURL = %q, want %q", got.EBirdURL, want)
	}
	if got.BirdsOfTheWorldURL != "https://birdsoftheworld.org/bow/species/whistor1/" {
		t.Errorf("BirdsOfTheWorldURL = %q, want the stored value verbatim", got.BirdsOfTheWorldURL)
	}

	if got.Photo == nil {
		t.Fatal("Photo = nil, want the rank-0 photo")
	}
	if got.Photo.ImageURL != "https://example.org/stork-best.jpg" {
		t.Errorf("Photo.ImageURL = %q, want the best-ranked photo, not the second one", got.Photo.ImageURL)
	}

	if len(got.Seasonality) != 2 || got.Seasonality[0].Week != 18 || got.Seasonality[1].Week != 19 {
		t.Errorf("Seasonality = %+v, want weeks 18 and 19 in order", got.Seasonality)
	}
}

// TestSpeciesDetailNoPhoto guards the nil case: a species with no
// species_media rows yet must not turn into a nil-pointer panic when the
// caller marshals it.
func TestSpeciesDetailNoPhoto(t *testing.T) {
	t.Parallel()

	fake := fakeStore{
		speciesRow: store.Species{ID: 99, ScientificName: "Rallus aquaticus"},
	}

	got, err := NewService(fake).SpeciesDetail(context.Background(), 1, 99, "lt")
	if err != nil {
		t.Fatalf("SpeciesDetail() error = %v", err)
	}
	if got.Photo != nil {
		t.Errorf("Photo = %+v, want nil (no species_media rows)", got.Photo)
	}
	if got.CommonName != "Rallus aquaticus" {
		t.Errorf("CommonName = %q, want scientific-name fallback %q", got.CommonName, "Rallus aquaticus")
	}
}

func TestSpeciesDetailPropagatesStoreError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("db down")
	_, err := NewService(fakeStore{speciesErr: wantErr}).SpeciesDetail(context.Background(), 1, 10, "lt")
	if !errors.Is(err, wantErr) {
		t.Errorf("SpeciesDetail() error = %v, want wrapping %v", err, wantErr)
	}
}

func TestVerificationURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		source    string
		sourceRef string
		want      string
	}{
		{name: "ebird", source: "ebird", sourceRef: "L123456", want: "https://ebird.org/hotspot/L123456"},
		{name: "unknown source", source: "manual", sourceRef: "1", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := verificationURL(tc.source, tc.sourceRef); got != tc.want {
				t.Errorf("verificationURL(%q, %q) = %q, want %q", tc.source, tc.sourceRef, got, tc.want)
			}
		})
	}
}
