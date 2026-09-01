package avilist

import (
	"bytes"
	"encoding/csv"
	"slices"
	"testing"
)

func TestRowToParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		record  []string
		wantErr bool
	}{
		{
			name:   "full row",
			record: []string{"comcra", "Grus grus", "avibase-99B8841E", "LC", "Gruidae", "Gruiformes", "https://bow.example/comcra", "https://datazone.example/comcra"},
		},
		{
			name:   "missing ebird code, still valid",
			record: []string{"", "Struthio molybdophanes", "avibase-40329BB6", "VU", "Struthionidae", "Struthioniformes", "https://bow.example/ostric3", ""},
		},
		{
			name:    "missing avibase_id",
			record:  []string{"comcra", "Grus grus", "", "LC", "Gruidae", "Gruiformes", "", ""},
			wantErr: true,
		},
		{
			name:    "missing scientific_name",
			record:  []string{"comcra", "", "avibase-99B8841E", "LC", "Gruidae", "Gruiformes", "", ""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			params, err := rowToParams(tt.record)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("rowToParams(%v) = nil error, want one", tt.record)
				}
				return
			}
			if err != nil {
				t.Fatalf("rowToParams(%v) = %v, want no error", tt.record, err)
			}

			if params.AvibaseID.String != tt.record[2] || !params.AvibaseID.Valid {
				t.Errorf("AvibaseID = %+v, want %q", params.AvibaseID, tt.record[2])
			}
			if params.ScientificName != tt.record[1] {
				t.Errorf("ScientificName = %q, want %q", params.ScientificName, tt.record[1])
			}
			wantEbirdCode := tt.record[0]
			if params.EbirdSpeciesCode.Valid != (wantEbirdCode != "") || params.EbirdSpeciesCode.String != wantEbirdCode {
				t.Errorf("EbirdSpeciesCode = %+v, want %q (valid=%v)", params.EbirdSpeciesCode, wantEbirdCode, wantEbirdCode != "")
			}
		})
	}
}

func TestEmbeddedCSVHeader(t *testing.T) {
	t.Parallel()

	// Guards the embedded CSV itself, not just the parser: if the checked-in
	// file's header ever drifts from wantHeader, this fails before Load ever
	// touches a database.
	header, err := csv.NewReader(bytes.NewReader(speciesCSV)).Read()
	if err != nil {
		t.Fatalf("reading embedded CSV header: %v", err)
	}
	if !slices.Equal(header, wantHeader) {
		t.Errorf("embedded CSV header = %v, want %v", header, wantHeader)
	}
}
