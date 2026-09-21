// Package ebd loads the eBird Basic Dataset -- a tab-separated export of
// individual observations -- into the checklists and observations tables. It
// also loads the export's sampling event file, which lists every checklist
// (including those with no observation rows) and nothing else.
package ebd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Kontora20/where-to-bird-today/internal/store"
)

// Source is the ingestion_runs.source value this loader writes.
const Source = "ebd"

// maxLineSize bounds one EBD row. The largest fields are free-text comments;
// 10MB is far beyond any real checklist comment, but cheap to allow so a
// single unusually long row fails loudly instead of stopping the whole load.
const maxLineSize = 10 * 1024 * 1024

// Stats summarizes one Load call.
type Stats struct {
	// Checklists is the number of distinct checklists upserted.
	Checklists int
	// Inserted is the number of observations upserted.
	Inserted int
	// SkippedUnmatchedSpecies counts species-category rows whose
	// TAXON CONCEPT ID has no matching species -- taxonomic drift between
	// AviList and eBird's live taxonomy, not a load failure.
	SkippedUnmatchedSpecies int
	// SkippedFiltered counts rows that are neither species-rank nor
	// approved -- excluded by design, not a data problem.
	SkippedFiltered int
}

// errSpeciesNotFound marks a row whose avibase_id has no matching species.
// It happens often enough (taxonomic drift, rare vagrants) that it must not
// abort the load; Load counts it instead of failing.
var errSpeciesNotFound = errors.New("no matching species for avibase_id")

// checklistColumns are read from every file: the main export and the
// sampling event file both carry them. Columns are read by name rather than
// position because this is an external vendor export whose column order and
// count have changed across releases (the main file has 52 columns; we only
// need these).
var checklistColumns = []string{
	"SAMPLING EVENT IDENTIFIER", "GROUP IDENTIFIER", "LOCALITY ID",
	"OBSERVATION DATE", "TIME OBSERVATIONS STARTED", "ALL SPECIES REPORTED",
}

// observationColumns are read only from the main export.
var observationColumns = []string{
	"GLOBAL UNIQUE IDENTIFIER", "CATEGORY", "TAXON CONCEPT ID",
	"OBSERVATION COUNT", "LATITUDE", "LONGITUDE", "APPROVED",
}

// Load streams an EBD file from r. A main export upserts every checklist it
// mentions and every approved, species-rank row as an observation. A
// sampling event file, recognised by having no observation columns, upserts
// checklists only. Stats reflects rows processed so far even when Load
// returns an error.
func Load(ctx context.Context, queries *store.Queries, r io.Reader) (Stats, error) {
	speciesByAvibaseID, err := loadSpeciesIndex(ctx, queries)
	if err != nil {
		return Stats{}, fmt.Errorf("loading species index: %w", err)
	}

	hotspotsByRef, err := loadHotspotIndex(ctx, queries)
	if err != nil {
		return Stats{}, fmt.Errorf("loading hotspot index: %w", err)
	}

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	// EBD is tab-separated but has no quoting convention at all: a `"` in a
	// free-text comment field is a literal character, never a delimiter or
	// an escape marker. encoding/csv doesn't know that -- an unescaped quote
	// makes it enter "inside a quoted field" state, which then silently
	// swallows following tabs and even newlines as literal content until it
	// finds another quote, merging physical rows into one and losing data
	// with no error at all. Splitting each line on tabs by hand sidesteps
	// CSV's quoting rules entirely, which is exactly what a quote-free
	// format needs.
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return Stats{}, fmt.Errorf("reading header: %w", err)
		}
		return Stats{}, errors.New("reading header: file is empty")
	}
	header := strings.Split(scanner.Text(), "\t")
	hasObservations := slices.Contains(header, "GLOBAL UNIQUE IDENTIFIER")
	want := checklistColumns
	if hasObservations {
		want = slices.Concat(checklistColumns, observationColumns)
	}
	cols, err := columnIndex(header, want)
	if err != nil {
		return Stats{}, err
	}

	var stats Stats
	checklistIDs := make(map[string]int64)
	for line := 2; scanner.Scan(); line++ {
		record := strings.Split(scanner.Text(), "\t")
		if len(record) != len(header) {
			return stats, fmt.Errorf("row %d: got %d fields, want %d", line, len(record), len(header))
		}

		// Every row registers its checklist first, whatever happens to the
		// observation: a checklist whose only rows are filtered out, or whose
		// species is unmatched, still counts in the denominator of every
		// frequency at its hotspot.
		samplingEventID := record[cols["SAMPLING EVENT IDENTIFIER"]]
		checklistID, seen := checklistIDs[samplingEventID]
		if !seen {
			checklist, err := checklistParams(record, cols, hotspotsByRef)
			if err != nil {
				return stats, fmt.Errorf("row %d: %w", line, err)
			}
			checklistID, err = queries.UpsertChecklist(ctx, checklist)
			if err != nil {
				return stats, fmt.Errorf("row %d (checklist %s): %w", line, samplingEventID, err)
			}
			checklistIDs[samplingEventID] = checklistID
			stats.Checklists++
		}
		if !hasObservations {
			continue
		}

		params, err := rowToParams(record, cols, checklistID, speciesByAvibaseID, hotspotsByRef)
		if errors.Is(err, errSpeciesNotFound) {
			stats.SkippedUnmatchedSpecies++
			continue
		}
		if err != nil {
			return stats, fmt.Errorf("row %d: %w", line, err)
		}
		if params == nil {
			stats.SkippedFiltered++
			continue
		}

		if _, err := queries.UpsertObservation(ctx, *params); err != nil {
			return stats, fmt.Errorf("row %d (source_ref %s): %w", line, params.SourceRef, err)
		}
		stats.Inserted++
	}
	if err := scanner.Err(); err != nil {
		return stats, fmt.Errorf("reading EBD export: %w", err)
	}

	return stats, nil
}

func loadSpeciesIndex(ctx context.Context, queries *store.Queries) (map[string]int64, error) {
	rows, err := queries.ListSpeciesAvibaseIDs(ctx)
	if err != nil {
		return nil, err
	}
	index := make(map[string]int64, len(rows))
	for _, row := range rows {
		index[row.AvibaseID.String] = row.ID
	}
	return index, nil
}

func loadHotspotIndex(ctx context.Context, queries *store.Queries) (map[string]int64, error) {
	rows, err := queries.ListHotspotRefsBySource(ctx, "ebird")
	if err != nil {
		return nil, err
	}
	index := make(map[string]int64, len(rows))
	for _, row := range rows {
		index[row.SourceRef] = row.ID
	}
	return index, nil
}

// columnIndex maps each name in want to its position in header, failing
// loudly if the export is missing a column Load needs.
func columnIndex(header []string, want []string) (map[string]int, error) {
	index := make(map[string]int, len(header))
	for position, name := range header {
		index[name] = position
	}
	for _, name := range want {
		if _, ok := index[name]; !ok {
			return nil, fmt.Errorf("EBD export is missing column %q", name)
		}
	}
	return index, nil
}

// checklistParams turns one EBD row into checklist upsert params. Every row
// of one checklist carries the same values, so the first one seen is enough.
func checklistParams(record []string, cols map[string]int, hotspotsByRef map[string]int64) (store.UpsertChecklistParams, error) {
	field := func(name string) string { return record[cols[name]] }

	observedOn, err := time.Parse("2006-01-02", field("OBSERVATION DATE"))
	if err != nil {
		return store.UpsertChecklistParams{}, fmt.Errorf("parsing observation date: %w", err)
	}
	startedAt, err := parseStartTime(field("TIME OBSERVATIONS STARTED"))
	if err != nil {
		return store.UpsertChecklistParams{}, err
	}
	hotspotID, hasHotspot := hotspotsByRef[field("LOCALITY ID")]
	group := field("GROUP IDENTIFIER")

	return store.UpsertChecklistParams{
		SamplingEventID:    field("SAMPLING EVENT IDENTIFIER"),
		GroupID:            pgtype.Text{String: group, Valid: group != ""},
		HotspotID:          pgtype.Int8{Int64: hotspotID, Valid: hasHotspot},
		ObservedOn:         pgtype.Date{Time: observedOn, Valid: true},
		StartedAt:          startedAt,
		AllSpeciesReported: field("ALL SPECIES REPORTED") == "1",
	}, nil
}

// parseStartTime reads TIME OBSERVATIONS STARTED, local time as HH:MM:SS.
// Some checklists have none, which is NULL rather than midnight.
func parseStartTime(raw string) (pgtype.Time, error) {
	if raw == "" {
		return pgtype.Time{}, nil
	}
	parsed, err := time.Parse("15:04:05", raw)
	if err != nil {
		return pgtype.Time{}, fmt.Errorf("parsing start time: %w", err)
	}
	sinceMidnight := time.Duration(parsed.Hour())*time.Hour + time.Duration(parsed.Minute())*time.Minute + time.Duration(parsed.Second())*time.Second
	return pgtype.Time{Microseconds: sinceMidnight.Microseconds(), Valid: true}, nil
}

// rowToParams turns one EBD row into observation upsert params. It returns
// (nil, nil) for a row Load should silently skip (not species-rank, or not
// approved), and errSpeciesNotFound when the row's TAXON CONCEPT ID has no
// matching species.
func rowToParams(record []string, cols map[string]int, checklistID int64, speciesByAvibaseID, hotspotsByRef map[string]int64) (*store.UpsertObservationParams, error) {
	field := func(name string) string { return record[cols[name]] }

	if field("CATEGORY") != "species" || field("APPROVED") != "1" {
		return nil, nil
	}

	speciesID, ok := speciesByAvibaseID[field("TAXON CONCEPT ID")]
	if !ok {
		return nil, errSpeciesNotFound
	}

	latitude, err := strconv.ParseFloat(field("LATITUDE"), 64)
	if err != nil {
		return nil, fmt.Errorf("parsing latitude: %w", err)
	}
	longitude, err := strconv.ParseFloat(field("LONGITUDE"), 64)
	if err != nil {
		return nil, fmt.Errorf("parsing longitude: %w", err)
	}
	observedOn, err := time.Parse("2006-01-02", field("OBSERVATION DATE"))
	if err != nil {
		return nil, fmt.Errorf("parsing observation date: %w", err)
	}

	hotspotID, hasHotspot := hotspotsByRef[field("LOCALITY ID")]

	return &store.UpsertObservationParams{
		SpeciesID:   speciesID,
		HotspotID:   pgtype.Int8{Int64: hotspotID, Valid: hasHotspot},
		ChecklistID: pgtype.Int8{Int64: checklistID, Valid: true},
		Longitude:   longitude,
		Latitude:    latitude,
		ObservedOn:  pgtype.Date{Time: observedOn, Valid: true},
		Count:       parseCount(field("OBSERVATION COUNT")),
		Source:      Source,
		SourceRef:   field("GLOBAL UNIQUE IDENTIFIER"),
	}, nil
}

// parseCount reads OBSERVATION COUNT: a number, or "X" meaning present but
// not counted.
func parseCount(raw string) pgtype.Int4 {
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(parsed), Valid: true}
}
