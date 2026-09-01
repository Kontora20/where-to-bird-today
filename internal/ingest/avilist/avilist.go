// Package avilist loads the AviList species reference CSV into the species
// table.
package avilist

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Kontora20/where-to-bird-today/internal/store"
)

// Source is the ingestion_runs.source value this loader writes.
const Source = "avilist"

//go:embed avilist_species.csv
var speciesCSV []byte

// wantHeader pins the loader to the CSV's exact column order. A drifted
// header fails loudly instead of silently loading the wrong column into the
// wrong field.
var wantHeader = []string{
	"ebird_species_code", "scientific_name", "avibase_id", "iucn_category",
	"family", "taxon_order", "birds_of_the_world_url", "birdlife_datazone_url",
}

// Load parses the embedded CSV and upserts every row into species. It
// returns the number of rows upserted.
func Load(ctx context.Context, queries *store.Queries) (int, error) {
	reader := csv.NewReader(bytes.NewReader(speciesCSV))

	header, err := reader.Read()
	if err != nil {
		return 0, fmt.Errorf("reading header: %w", err)
	}
	if !slices.Equal(header, wantHeader) {
		return 0, fmt.Errorf("unexpected CSV header %v, want %v", header, wantHeader)
	}

	count := 0
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return count, fmt.Errorf("reading row %d: %w", count+2, err)
		}

		params, err := rowToParams(record)
		if err != nil {
			return count, fmt.Errorf("row %d: %w", count+2, err)
		}

		if _, err := queries.UpsertSpecies(ctx, params); err != nil {
			return count, fmt.Errorf("row %d (avibase_id %s): %w", count+2, params.AvibaseID.String, err)
		}
		count++
	}

	return count, nil
}

func rowToParams(record []string) (store.UpsertSpeciesParams, error) {
	avibaseID := record[2]
	if avibaseID == "" {
		return store.UpsertSpeciesParams{}, errors.New("missing avibase_id")
	}
	scientificName := record[1]
	if scientificName == "" {
		return store.UpsertSpeciesParams{}, errors.New("missing scientific_name")
	}

	return store.UpsertSpeciesParams{
		EbirdSpeciesCode:    optionalText(record[0]),
		ScientificName:      scientificName,
		AvibaseID:           pgtype.Text{String: avibaseID, Valid: true},
		IucnCategory:        optionalText(record[3]),
		Family:              optionalText(record[4]),
		TaxonOrder:          optionalText(record[5]),
		BirdsOfTheWorldUrl:  optionalText(record[6]),
		BirdlifeDatazoneUrl: optionalText(record[7]),
	}, nil
}

// optionalText turns an empty CSV field into SQL NULL rather than an empty
// string.
func optionalText(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}
