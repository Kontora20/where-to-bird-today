package birding

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

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
			{WeekOfYear: 18, Frequency: 0.4, SampleSize: 10},
			{WeekOfYear: 19, Frequency: 0.6, SampleSize: 5},
		},
		latestRow: &store.GetLatestChecklistWithSpeciesRow{
			SamplingEventID: "S123456",
			ObservedOn:      pgtype.Date{Time: time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC), Valid: true},
		},
	}

	got, err := NewService(fake, nil).SpeciesDetail(context.Background(), 1, 10, "lt")
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
	if got.Seasonality[0].Checklists != 10 || got.Seasonality[1].Checklists != 5 {
		t.Errorf("Seasonality = %+v, want the checklist counts 10 and 5", got.Seasonality)
	}

	want := &LatestChecklist{URL: "https://ebird.org/checklist/S123456", ObservedOn: "2026-05-03"}
	if got.LatestChecklist == nil || *got.LatestChecklist != *want {
		t.Errorf("LatestChecklist = %+v, want %+v", got.LatestChecklist, want)
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

	got, err := NewService(fake, nil).SpeciesDetail(context.Background(), 1, 99, "lt")
	if err != nil {
		t.Fatalf("SpeciesDetail() error = %v", err)
	}
	if got.Photo != nil {
		t.Errorf("Photo = %+v, want nil (no species_media rows)", got.Photo)
	}
	if got.LatestChecklist != nil {
		t.Errorf("LatestChecklist = %+v, want nil (species never recorded at the hotspot)", got.LatestChecklist)
	}
	if got.CommonName != "Rallus aquaticus" {
		t.Errorf("CommonName = %q, want scientific-name fallback %q", got.CommonName, "Rallus aquaticus")
	}
}

func TestSpeciesDetailPropagatesStoreError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("db down")
	_, err := NewService(fakeStore{speciesErr: wantErr}, nil).SpeciesDetail(context.Background(), 1, 10, "lt")
	if !errors.Is(err, wantErr) {
		t.Errorf("SpeciesDetail() error = %v, want wrapping %v", err, wantErr)
	}
}

func TestSpeciesDetailPropagatesLatestChecklistError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("db down")
	fake := fakeStore{speciesRow: store.Species{ID: 10, ScientificName: "Ciconia ciconia"}, latestErr: wantErr}
	_, err := NewService(fake, nil).SpeciesDetail(context.Background(), 1, 10, "lt")
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
