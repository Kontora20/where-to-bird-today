package birding

import (
	"context"
	"errors"
	"slices"
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
		seasonalityRows: yearOfWeeks(map[int16]store.GetHotspotSpeciesWeeksRow{
			18: {Checklists: 10, WithSpecies: 4},
			19: {Checklists: 5, WithSpecies: 3},
		}),
		latestRow: &store.GetLatestChecklistWithSpeciesRow{
			SamplingEventID: "S123456",
			ObservedOn:      pgtype.Date{Time: time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC), Valid: true},
		},
	}

	got, err := NewService(fake, nil).SpeciesDetail(context.Background(), 1, 10, "lt", time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC))
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

	if len(got.Seasonality) != weeksPerYear {
		t.Fatalf("len(Seasonality) = %d, want every week of the year", len(got.Seasonality))
	}
	if week18, week19 := got.Seasonality[17], got.Seasonality[18]; week18.Week != 18 || week18.Checklists != 10 || week18.Frequency != 0.4 || week19.Week != 19 || week19.Checklists != 5 || week19.Frequency != 0.6 {
		t.Errorf("weeks 18 and 19 = %+v and %+v, want 4 of 10 and 3 of 5 checklists", week18, week19)
	}
	// Weeks 18 and 19 each pool 15 checklists, 7 with the species, and week 20
	// pools 5 with 3 (3 of 5 at week 19): all within a tenth of the highest,
	// one run from 27 April to 17 May 2026.
	wantBest := []WeekRange{{FromWeek: 18, ToWeek: 20, Start: "2026-04-27", End: "2026-05-17"}}
	if !slices.Equal(got.BestWeeks, wantBest) {
		t.Errorf("BestWeeks = %+v, want %+v", got.BestWeeks, wantBest)
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

	got, err := NewService(fake, nil).SpeciesDetail(context.Background(), 1, 99, "lt", time.Time{})
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
	_, err := NewService(fakeStore{speciesErr: wantErr}, nil).SpeciesDetail(context.Background(), 1, 10, "lt", time.Time{})
	if !errors.Is(err, wantErr) {
		t.Errorf("SpeciesDetail() error = %v, want wrapping %v", err, wantErr)
	}
}

func TestSpeciesDetailPropagatesLatestChecklistError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("db down")
	fake := fakeStore{speciesRow: store.Species{ID: 10, ScientificName: "Ciconia ciconia"}, latestErr: wantErr}
	_, err := NewService(fake, nil).SpeciesDetail(context.Background(), 1, 10, "lt", time.Time{})
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
