package ebd

import (
	"errors"
	"testing"
)

var testHeader = []string{
	"GLOBAL UNIQUE IDENTIFIER", "CATEGORY", "TAXON CONCEPT ID",
	"OBSERVATION COUNT", "LOCALITY ID", "LATITUDE", "LONGITUDE",
	"OBSERVATION DATE", "APPROVED",
}

func testRecord(overrides map[string]string) []string {
	values := map[string]string{
		"GLOBAL UNIQUE IDENTIFIER": "URN:CornellLabOfOrnithology:EBIRD:OBS1",
		"CATEGORY":                 "species",
		"TAXON CONCEPT ID":         "avibase-CA5852BE",
		"OBSERVATION COUNT":        "3",
		"LOCALITY ID":              "L16048837",
		"LATITUDE":                 "54.3133749",
		"LONGITUDE":                "23.6597231",
		"OBSERVATION DATE":         "2025-01-10",
		"APPROVED":                 "1",
	}
	for name, value := range overrides {
		values[name] = value
	}

	record := make([]string, len(testHeader))
	for i, name := range testHeader {
		record[i] = values[name]
	}
	return record
}

func TestRowToParams(t *testing.T) {
	t.Parallel()

	cols, err := columnIndex(testHeader, requiredColumns)
	if err != nil {
		t.Fatalf("columnIndex: %v", err)
	}
	speciesByAvibaseID := map[string]int64{"avibase-CA5852BE": 42}
	hotspotsByRef := map[string]int64{"L16048837": 7}

	tests := []struct {
		name            string
		overrides       map[string]string
		wantSkip        bool
		wantSpeciesMiss bool
		wantHardErr     bool
		wantHotspot     bool
		wantCount       bool
	}{
		{
			name:        "hotspot locality, counted",
			wantHotspot: true,
			wantCount:   true,
		},
		{
			name:      "personal locality, no hotspot match",
			overrides: map[string]string{"LOCALITY ID": "L99999999"},
			wantCount: true,
		},
		{
			name:        "X count means present, uncounted",
			overrides:   map[string]string{"OBSERVATION COUNT": "X"},
			wantHotspot: true,
			wantCount:   false,
		},
		{
			name:      "non-species category is skipped",
			overrides: map[string]string{"CATEGORY": "spuh"},
			wantSkip:  true,
		},
		{
			name:      "unapproved record is skipped",
			overrides: map[string]string{"APPROVED": "0"},
			wantSkip:  true,
		},
		{
			name:            "unmatched avibase_id",
			overrides:       map[string]string{"TAXON CONCEPT ID": "avibase-00000000"},
			wantSpeciesMiss: true,
		},
		{
			name:        "malformed date is a hard error",
			overrides:   map[string]string{"OBSERVATION DATE": "10/01/2025"},
			wantHardErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			params, err := rowToParams(testRecord(tt.overrides), cols, speciesByAvibaseID, hotspotsByRef)

			if tt.wantSkip {
				if params != nil || err != nil {
					t.Fatalf("rowToParams() = %+v, %v, want (nil, nil)", params, err)
				}
				return
			}
			if tt.wantSpeciesMiss {
				if !errors.Is(err, errSpeciesNotFound) {
					t.Fatalf("rowToParams() error = %v, want errSpeciesNotFound", err)
				}
				return
			}
			if tt.wantHardErr {
				if err == nil || errors.Is(err, errSpeciesNotFound) {
					t.Fatalf("rowToParams() error = %v, want a non-sentinel error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("rowToParams() = %v, want no error", err)
			}

			if params.HotspotID.Valid != tt.wantHotspot {
				t.Errorf("HotspotID = %+v, want valid=%v", params.HotspotID, tt.wantHotspot)
			}
			if params.Count.Valid != tt.wantCount {
				t.Errorf("Count = %+v, want valid=%v", params.Count, tt.wantCount)
			}
			if params.SpeciesID != 42 {
				t.Errorf("SpeciesID = %d, want 42", params.SpeciesID)
			}
		})
	}
}

func TestColumnIndexMissingColumn(t *testing.T) {
	t.Parallel()

	_, err := columnIndex([]string{"CATEGORY", "APPROVED"}, requiredColumns)
	if err == nil {
		t.Fatal("columnIndex() = nil error, want one for a header missing required columns")
	}
}
