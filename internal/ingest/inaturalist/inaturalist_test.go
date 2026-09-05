package inaturalist

import "testing"

// TestExactBirdMatchIgnoresFalsePositives is the regression test for the
// real bug this package's safety filter exists to prevent: searching
// iNaturalist for a bird's own scientific name is a fuzzy text search
// across every kingdom, and can rank a completely unrelated species above
// the actual match. Captured live against api.inaturalist.org: querying
// "Grus grus" (Common Crane) returns the crane's genuine record third,
// behind a Sandhill Crane synonym match and a puffball fungus matched via
// an unrelated Polish common name containing "grus".
func TestExactBirdMatchIgnoresFalsePositives(t *testing.T) {
	t.Parallel()

	results := []taxaSearchResult{
		{ID: 508048, Name: "Antigone canadensis", IconicTaxonName: "Aves"},   // Sandhill Crane, matched via synonym
		{ID: 972793, Name: "Apioperdon pyriforme", IconicTaxonName: "Fungi"}, // puffball fungus, matched via Polish name
		{ID: 41, Name: "Grus grus", IconicTaxonName: "Aves"},                 // the actual match, ranked third
	}

	id, found := exactBirdMatch(results, "Grus grus")
	if !found {
		t.Fatal("exactBirdMatch found nothing, want the crane at id 41")
	}
	if id != 41 {
		t.Errorf("exactBirdMatch id = %d, want 41 (the crane, not the fungus or the synonym match)", id)
	}
}

func TestExactBirdMatchNoMatchIsNotAGuess(t *testing.T) {
	t.Parallel()

	results := []taxaSearchResult{
		{ID: 1, Name: "Antigone canadensis", IconicTaxonName: "Aves"},
		{ID: 2, Name: "Apioperdon pyriforme", IconicTaxonName: "Fungi"},
	}

	// Neither result is an exact match for the query -- must report "not
	// found", never fall back to the top-ranked wrong result.
	if _, found := exactBirdMatch(results, "Grus grus"); found {
		t.Error("exactBirdMatch found a match, want none: no result exactly matches the query")
	}
}

func TestExactBirdMatchRejectsNonBirdExactNameMatch(t *testing.T) {
	t.Parallel()

	// Same scientific name, wrong kingdom -- must still be rejected even
	// though the name itself matches exactly.
	results := []taxaSearchResult{
		{ID: 99, Name: "Grus grus", IconicTaxonName: "Fungi"},
	}

	if _, found := exactBirdMatch(results, "Grus grus"); found {
		t.Error("exactBirdMatch matched a non-Aves result with the right name, want it rejected")
	}
}

func TestPhotosFromTaxonPhotosDropsUnlicensedAndCaps(t *testing.T) {
	t.Parallel()

	entries := []taxonPhoto{
		{ID: 1, LicenseCode: "cc-by-nc-sa", Attribution: "a"},
		{ID: 2, LicenseCode: "", Attribution: "no licence, must be dropped"},
		{ID: 3, LicenseCode: "cc-by-nc", Attribution: "b"},
		{ID: 4, LicenseCode: "cc-by-nc", Attribution: "c"},
		{ID: 5, LicenseCode: "cc-by", Attribution: "d, beyond the cap"},
	}

	photos, err := photosFromTaxonPhotos(entries)
	if err != nil {
		t.Fatalf("photosFromTaxonPhotos: %v", err)
	}

	if len(photos) != maxPhotosPerSpecies {
		t.Fatalf("photosFromTaxonPhotos returned %d photos, want %d (capped, one entry dropped for no licence)", len(photos), maxPhotosPerSpecies)
	}
	for _, photo := range photos {
		if photo.License == "" {
			t.Error("photosFromTaxonPhotos kept a photo with no licence")
		}
	}
}
