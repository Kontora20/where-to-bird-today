package store

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

const migrationsDir = "../../migrations"

// newTestPool starts a real Postgres+PostGIS container, migrates it, and
// returns a pool connected to it. Skipped under -short.
func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping Postgres-backed integration test in -short mode")
	}

	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx, "postgis/postgis:17-3.5",
		postgres.WithDatabase("wheretobird"),
		postgres.WithUsername("birder"),
		postgres.WithPassword("birder"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("starting postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := pgContainer.Terminate(context.Background()); err != nil {
			t.Errorf("terminating postgres container: %v", err)
		}
	})

	connString, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("getting connection string: %v", err)
	}

	sqlDB, err := sql.Open("pgx", connString)
	if err != nil {
		t.Fatalf("opening database/sql handle: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()

	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("setting goose dialect: %v", err)
	}
	if err := goose.Up(sqlDB, migrationsDir); err != nil {
		t.Fatalf("running migrations: %v", err)
	}

	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		t.Fatalf("opening pgx pool: %v", err)
	}
	t.Cleanup(pool.Close)

	return pool
}

func TestIngestionRunsRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	queries := New(newTestPool(t))

	created, err := queries.CreateIngestionRun(ctx, "ebird")
	if err != nil {
		t.Fatalf("CreateIngestionRun: %v", err)
	}
	if created.Source != "ebird" {
		t.Errorf("created.Source = %q, want %q", created.Source, "ebird")
	}
	if created.Status != "running" {
		t.Errorf("created.Status = %q, want %q", created.Status, "running")
	}

	got, err := queries.GetIngestionRun(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetIngestionRun: %v", err)
	}
	if got.ID != created.ID || got.Source != created.Source {
		t.Errorf("GetIngestionRun = %+v, want id=%d source=%q", got, created.ID, created.Source)
	}
}

func TestHotspotsRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	queries := New(newTestPool(t))

	const wantLatitude = 55.3167
	const wantLongitude = 21.3667

	created, err := queries.CreateHotspot(ctx, CreateHotspotParams{
		Name:      "Nemunas Delta",
		Latitude:  wantLatitude,
		Longitude: wantLongitude,
		Region:    "LT",
		Source:    "manual",
		SourceRef: "test-hotspot-1",
	})
	if err != nil {
		t.Fatalf("CreateHotspot: %v", err)
	}
	if created.Name != "Nemunas Delta" {
		t.Errorf("created.Name = %q, want %q", created.Name, "Nemunas Delta")
	}
	// The round trip goes through ST_MakePoint -> geography -> ST_X/ST_Y, so
	// this also proves the PostGIS coordinate handling works, not just that a
	// row was stored.
	if !floatsClose(created.Latitude.Float64, wantLatitude) {
		t.Errorf("created.Latitude = %v, want ~%v", created.Latitude.Float64, wantLatitude)
	}
	if !floatsClose(created.Longitude.Float64, wantLongitude) {
		t.Errorf("created.Longitude = %v, want ~%v", created.Longitude.Float64, wantLongitude)
	}

	got, err := queries.GetHotspot(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetHotspot: %v", err)
	}
	if got.ID != created.ID || got.Name != created.Name {
		t.Errorf("GetHotspot = %+v, want id=%d name=%q", got, created.ID, created.Name)
	}
}

// A species with no eBird code has a NULL ebird_species_code, and NULL
// never conflicts with itself in Postgres -- this checks that upserting on
// avibase_id instead still avoids a duplicate row on re-run.
func TestUpsertSpeciesIsIdempotentForNullEbirdCode(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	queries := New(newTestPool(t))

	params := UpsertSpeciesParams{
		ScientificName: "Struthio molybdophanes",
		AvibaseID:      pgtype.Text{String: "avibase-40329BB6", Valid: true},
		// EbirdSpeciesCode left zero-valued: Valid=false, i.e. SQL NULL.
	}

	first, err := queries.UpsertSpecies(ctx, params)
	if err != nil {
		t.Fatalf("first UpsertSpecies: %v", err)
	}

	second, err := queries.UpsertSpecies(ctx, params)
	if err != nil {
		t.Fatalf("second UpsertSpecies: %v", err)
	}

	if second.ID != first.ID {
		t.Errorf("second.ID = %d, want %d (re-run inserted a duplicate row)", second.ID, first.ID)
	}
}

func TestUpsertHotspotIsIdempotent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	queries := New(newTestPool(t))

	params := UpsertHotspotParams{
		Name:      "Alka Polder",
		Longitude: 21.3721427,
		Latitude:  55.4382263,
		Region:    "LT",
		Source:    "ebird",
		SourceRef: "L14634126",
	}

	first, err := queries.UpsertHotspot(ctx, params)
	if err != nil {
		t.Fatalf("first UpsertHotspot: %v", err)
	}

	params.Name = "Alka Polder (renamed)"
	second, err := queries.UpsertHotspot(ctx, params)
	if err != nil {
		t.Fatalf("second UpsertHotspot: %v", err)
	}

	if second.ID != first.ID {
		t.Errorf("second.ID = %d, want %d (re-run inserted a duplicate row)", second.ID, first.ID)
	}
	if second.Name != "Alka Polder (renamed)" {
		t.Errorf("second.Name = %q, want the updated name", second.Name)
	}
}

// TestRecomputeSpeciesSeasonality seeds one ISO week at one hotspot and
// checks the recomputed frequency and sample_size by hand. Complete
// checklists there: S1 (species A and B), S2 (A), S3 (no species, so it
// only counts in the denominator), and a shared checklist submitted twice as
// S4 and S5 (species B, both copies). Around them sit rows that must not
// count: S6 is incomplete (species C) and S7 is at a personal location
// (species A). Four checklists make the denominator, because the two shared
// copies are one checklist: A is on 2 of 4 (0.5), B on S1 and the shared one
// (0.5, not 3 of 5), C nowhere. It then deletes S3 and recomputes: a stale
// sample_size of 4 surviving would mean the table was appended to, not
// rebuilt.
//
// A second hotspot in the same country has one complete checklist with
// species A, which sets the country's frequency the hotspots' adjusted
// frequencies are pulled toward: with a prior of 5 checklists, A at the
// first hotspot is (2 + 5*0.6) / (4 + 5) and at the second (1 + 5*0.6) / (1 + 5).
func TestRecomputeSpeciesSeasonality(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newTestPool(t)
	queries := New(pool)

	hotspot, err := queries.UpsertHotspot(ctx, UpsertHotspotParams{
		Name:      "Seasonality Test Hotspot",
		Longitude: 25.0,
		Latitude:  55.0,
		Region:    "LT",
		Source:    "test",
		SourceRef: "test-seasonality-hotspot",
	})
	if err != nil {
		t.Fatalf("UpsertHotspot: %v", err)
	}

	seedSpecies := func(scientificName, avibaseID string) int64 {
		t.Helper()
		species, err := queries.UpsertSpecies(ctx, UpsertSpeciesParams{
			ScientificName: scientificName,
			AvibaseID:      pgtype.Text{String: avibaseID, Valid: true},
		})
		if err != nil {
			t.Fatalf("UpsertSpecies (%s): %v", scientificName, err)
		}
		return species.ID
	}
	otherHotspot, err := queries.UpsertHotspot(ctx, UpsertHotspotParams{
		Name:      "Seasonality Test Other Hotspot",
		Longitude: 25.5,
		Latitude:  55.5,
		Region:    "LT",
		Source:    "test",
		SourceRef: "test-seasonality-other-hotspot",
	})
	if err != nil {
		t.Fatalf("UpsertHotspot (other): %v", err)
	}

	speciesA := seedSpecies("Testus speciesa", "avibase-TESTAAA1")
	speciesB := seedSpecies("Testus speciesb", "avibase-TESTBBB1")
	speciesC := seedSpecies("Testus speciesc", "avibase-TESTCCC1")
	speciesD := seedSpecies("Testus speciesd", "avibase-TESTDDD1")

	seedChecklist := func(samplingEventID, groupID string, atHotspot bool, day int, complete bool) int64 {
		t.Helper()
		params := UpsertChecklistParams{
			SamplingEventID:    samplingEventID,
			GroupID:            pgtype.Text{String: groupID, Valid: groupID != ""},
			ObservedOn:         pgtype.Date{Time: time.Date(2026, 3, day, 0, 0, 0, 0, time.UTC), Valid: true},
			AllSpeciesReported: complete,
		}
		if atHotspot {
			params.HotspotID = pgtype.Int8{Int64: hotspot.ID, Valid: true}
		}
		id, err := queries.UpsertChecklist(ctx, params)
		if err != nil {
			t.Fatalf("seeding checklist %s: %v", samplingEventID, err)
		}
		return id
	}
	seedObservation := func(checklistID, speciesID int64, atHotspot bool, day int, sourceRef string) {
		t.Helper()
		params := UpsertObservationParams{
			SpeciesID:   speciesID,
			ChecklistID: pgtype.Int8{Int64: checklistID, Valid: true},
			Longitude:   25.0,
			Latitude:    55.0,
			ObservedOn:  pgtype.Date{Time: time.Date(2026, 3, day, 0, 0, 0, 0, time.UTC), Valid: true},
			Source:      "test",
			SourceRef:   sourceRef,
		}
		if atHotspot {
			params.HotspotID = pgtype.Int8{Int64: hotspot.ID, Valid: true}
		}
		if _, err := queries.UpsertObservation(ctx, params); err != nil {
			t.Fatalf("seeding observation %s: %v", sourceRef, err)
		}
	}

	// All within the same ISO week: Monday 2026-03-02 to Friday 2026-03-06.
	s1 := seedChecklist("S1", "", true, 2, true)
	s2 := seedChecklist("S2", "", true, 3, true)
	s3 := seedChecklist("S3", "", true, 4, true)
	s4 := seedChecklist("S4", "G1", true, 5, true)
	s5 := seedChecklist("S5", "G1", true, 5, true)
	s6 := seedChecklist("S6", "", true, 5, false)
	s7 := seedChecklist("S7", "", false, 6, true)
	seedObservation(s1, speciesA, true, 2, "obs-s1-a")
	seedObservation(s1, speciesB, true, 2, "obs-s1-b")
	seedObservation(s2, speciesA, true, 3, "obs-s2-a")
	seedObservation(s4, speciesB, true, 5, "obs-s4-b")
	seedObservation(s5, speciesB, true, 5, "obs-s5-b")
	seedObservation(s6, speciesC, true, 5, "obs-s6-c")
	seedObservation(s7, speciesA, false, 6, "obs-s7-a")

	s8, err := queries.UpsertChecklist(ctx, UpsertChecklistParams{
		SamplingEventID:    "S8",
		HotspotID:          pgtype.Int8{Int64: otherHotspot.ID, Valid: true},
		ObservedOn:         pgtype.Date{Time: time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC), Valid: true},
		AllSpeciesReported: true,
	})
	if err != nil {
		t.Fatalf("seeding checklist S8: %v", err)
	}
	if _, err := queries.UpsertObservation(ctx, UpsertObservationParams{
		SpeciesID:   speciesA,
		HotspotID:   pgtype.Int8{Int64: otherHotspot.ID, Valid: true},
		ChecklistID: pgtype.Int8{Int64: s8, Valid: true},
		Longitude:   25.5,
		Latitude:    55.5,
		ObservedOn:  pgtype.Date{Time: time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC), Valid: true},
		Source:      "test",
		SourceRef:   "obs-s8-a",
	}); err != nil {
		t.Fatalf("seeding observation obs-s8-a: %v", err)
	}

	recompute := func() {
		t.Helper()
		if err := queries.TruncateSeasonality(ctx); err != nil {
			t.Fatalf("TruncateSeasonality: %v", err)
		}
		if _, err := queries.RecomputeSpeciesSeasonality(ctx, 5); err != nil {
			t.Fatalf("RecomputeSpeciesSeasonality: %v", err)
		}
	}
	recompute()

	_, weekOfYear := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC).ISOWeek()

	assertSeasonality(ctx, t, pool, hotspot.ID, speciesA, weekOfYear, 2.0/4.0, 4, 5.0/9.0)
	assertSeasonality(ctx, t, pool, hotspot.ID, speciesB, weekOfYear, 2.0/4.0, 4, 4.0/9.0)
	assertSeasonality(ctx, t, pool, otherHotspot.ID, speciesA, weekOfYear, 1.0, 1, 4.0/6.0)

	var rowsForC int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM species_seasonality WHERE species_id = $1", speciesC).Scan(&rowsForC); err != nil {
		t.Fatalf("counting rows for the incomplete-checklist species: %v", err)
	}
	if rowsForC != 0 {
		t.Errorf("species seen only on an incomplete checklist has %d seasonality rows, want 0", rowsForC)
	}

	if _, err := pool.Exec(ctx, "DELETE FROM checklists WHERE id = $1", s3); err != nil {
		t.Fatalf("removing checklist: %v", err)
	}
	recompute()

	assertSeasonality(ctx, t, pool, hotspot.ID, speciesA, weekOfYear, 2.0/3.0, 3, 5.75/8.0)
	assertSeasonality(ctx, t, pool, hotspot.ID, speciesB, weekOfYear, 2.0/3.0, 3, 4.5/8.0)

	// The latest checklist that recorded species A at the hotspot is S2 (the
	// personal-location S7 is later but not at this hotspot). A species never
	// recorded there has none.
	latest, err := queries.GetLatestChecklistWithSpecies(ctx, GetLatestChecklistWithSpeciesParams{SpeciesID: speciesA, HotspotID: pgtype.Int8{Int64: hotspot.ID, Valid: true}})
	if err != nil {
		t.Fatalf("GetLatestChecklistWithSpecies: %v", err)
	}
	if latest.SamplingEventID != "S2" {
		t.Errorf("latest checklist for species A = %q, want S2", latest.SamplingEventID)
	}
	if _, err := queries.GetLatestChecklistWithSpecies(ctx, GetLatestChecklistWithSpeciesParams{SpeciesID: speciesD, HotspotID: pgtype.Int8{Int64: hotspot.ID, Valid: true}}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("GetLatestChecklistWithSpecies for an unrecorded species error = %v, want pgx.ErrNoRows", err)
	}
}

// TestTimeOfDay seeds checklists with known minutes after sunrise and checks
// the counts and the adjusted frequency by hand. Hotspot A (Lithuania) has,
// in hour 0: a checklist with the species, one without, and a shared
// checklist submitted twice with the species (one checklist, not two): 3
// checklists, 2 with the species. In hour 1 it has one checklist with the
// species. Around them sit rows that must not count: an incomplete
// checklist and one with no minutes. Hotspot B (Lithuania) has one checklist
// without the species in hour 0 and one with it in hour 2. Hotspot C
// (Latvia) has one with the species in hour 0. The region is all three
// hotspots, whatever their country: in hour 0 it is 3 of 5 (0.6), in hours 1
// and 2 it is 1 of 1, and over all hours it is 5 of 7. A region prior of 3
// checklists pulls each hour's share toward the 5/7: hour 0 becomes
// (3 + 3*5/7) / (5 + 3), hours 1 and 2 (1 + 3*5/7) / (1 + 3). A prior of 5
// checklists then pulls A's hour 0 to (2 + 5*0.642857) / (3 + 5), its hour 1
// to (1 + 5*0.785714) / (1 + 5), and its hour 2, where A has no checklists,
// is the region's 0.785714 alone.
func TestTimeOfDay(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newTestPool(t)
	queries := New(pool)

	seedHotspot := func(name, region string) Hotspot {
		t.Helper()
		row, err := queries.UpsertHotspot(ctx, UpsertHotspotParams{
			Name: name, Longitude: 25.0, Latitude: 55.0, Region: region, Source: "test", SourceRef: "test-tod-" + name,
		})
		if err != nil {
			t.Fatalf("UpsertHotspot (%s): %v", name, err)
		}
		return Hotspot{ID: row.ID}
	}
	hotspotA := seedHotspot("a", "LT")
	hotspotB := seedHotspot("b", "LT")
	hotspotC := seedHotspot("c", "LV")

	species, err := queries.UpsertSpecies(ctx, UpsertSpeciesParams{
		ScientificName: "Testus nocturnus",
		AvibaseID:      pgtype.Text{String: "avibase-TESTTOD1", Valid: true},
	})
	if err != nil {
		t.Fatalf("UpsertSpecies: %v", err)
	}

	day := pgtype.Date{Time: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), Valid: true}
	seedChecklist := func(samplingEventID, groupID string, hotspot Hotspot, complete bool, minutes any, withSpecies bool) {
		t.Helper()
		id, err := queries.UpsertChecklist(ctx, UpsertChecklistParams{
			SamplingEventID:    samplingEventID,
			GroupID:            pgtype.Text{String: groupID, Valid: groupID != ""},
			HotspotID:          pgtype.Int8{Int64: hotspot.ID, Valid: true},
			ObservedOn:         day,
			AllSpeciesReported: complete,
		})
		if err != nil {
			t.Fatalf("UpsertChecklist (%s): %v", samplingEventID, err)
		}
		if _, err := pool.Exec(ctx, "UPDATE checklists SET minutes_after_sunrise = $2 WHERE id = $1", id, minutes); err != nil {
			t.Fatalf("setting minutes for %s: %v", samplingEventID, err)
		}
		if !withSpecies {
			return
		}
		if _, err := queries.UpsertObservation(ctx, UpsertObservationParams{
			SpeciesID:   species.ID,
			HotspotID:   pgtype.Int8{Int64: hotspot.ID, Valid: true},
			ChecklistID: pgtype.Int8{Int64: id, Valid: true},
			Longitude:   25.0,
			Latitude:    55.0,
			ObservedOn:  day,
			Source:      "test",
			SourceRef:   "obs-" + samplingEventID,
		}); err != nil {
			t.Fatalf("UpsertObservation (%s): %v", samplingEventID, err)
		}
	}

	seedChecklist("A1", "", hotspotA, true, 10, true)
	seedChecklist("A2", "", hotspotA, true, 30, false)
	seedChecklist("A3", "", hotspotA, true, 70, true)
	seedChecklist("A4", "G1", hotspotA, true, 20, true)
	seedChecklist("A5", "G1", hotspotA, true, 20, true)
	seedChecklist("A6", "", hotspotA, false, 15, true)
	seedChecklist("A7", "", hotspotA, true, nil, true)
	seedChecklist("B1", "", hotspotB, true, 5, false)
	seedChecklist("B2", "", hotspotB, true, 130, true)
	seedChecklist("C1", "", hotspotC, true, 5, true)

	if err := queries.TruncateTimeOfDay(ctx); err != nil {
		t.Fatalf("TruncateTimeOfDay: %v", err)
	}
	totals, err := queries.RecomputeTimeOfDayTotals(ctx)
	if err != nil {
		t.Fatalf("RecomputeTimeOfDayTotals: %v", err)
	}
	withSpecies, err := queries.RecomputeSpeciesTimeOfDay(ctx)
	if err != nil {
		t.Fatalf("RecomputeSpeciesTimeOfDay: %v", err)
	}
	// Totals: A hours 0 and 1, B hours 0 and 2, C hour 0. Species: A hours 0
	// and 1, B hour 2, C hour 0.
	if totals != 5 || withSpecies != 4 {
		t.Errorf("rows written = %d totals and %d with the species, want 5 and 4", totals, withSpecies)
	}

	read := func(hotspot Hotspot, minRegionChecklists int64) []GetSpeciesTimeOfDayRow {
		t.Helper()
		rows, err := queries.GetSpeciesTimeOfDay(ctx, GetSpeciesTimeOfDayParams{
			HotspotID:             pgtype.Int8{Int64: hotspot.ID, Valid: true},
			SpeciesID:             pgtype.Int8{Int64: species.ID, Valid: true},
			PriorChecklists:       pgtype.Float4{Float32: 5, Valid: true},
			RegionPriorChecklists: pgtype.Float4{Float32: 3, Valid: true},
			MinRegionChecklists:   pgtype.Int8{Int64: minRegionChecklists, Valid: true},
		})
		if err != nil {
			t.Fatalf("GetSpeciesTimeOfDay: %v", err)
		}
		return rows
	}
	check := func(got GetSpeciesTimeOfDayRow, hour int16, checklists, withSpecies int32, adjusted float64) {
		t.Helper()
		if got.HourBin != hour || got.Checklists != checklists || got.WithSpecies != withSpecies || math.Abs(float64(got.AdjustedFrequency)-adjusted) > 1e-4 {
			t.Errorf("hour %d = %d of %d, adjusted %v; want hour %d = %d of %d, adjusted %v",
				got.HourBin, got.WithSpecies, got.Checklists, got.AdjustedFrequency, hour, withSpecies, checklists, adjusted)
		}
	}

	t.Run("adjusts a hotspot toward its country", func(t *testing.T) {
		rows := read(hotspotA, 1)
		if len(rows) != 3 {
			t.Fatalf("got %d hours, want 3 (0, 1 and 2)", len(rows))
		}
		check(rows[0], 0, 3, 2, (2+5*(3+3*5.0/7)/8)/8)
		check(rows[1], 1, 1, 1, (1+5*(1+3*5.0/7)/4)/6)
		check(rows[2], 2, 0, 0, (1+3*5.0/7)/4)
	})

	t.Run("an hour the hotspot has no species in counts as none", func(t *testing.T) {
		rows := read(hotspotB, 1)
		check(rows[0], 0, 1, 0, (5*(3+3*5.0/7)/8)/6)
	})

	t.Run("leaves out hours the region has too few checklists in", func(t *testing.T) {
		rows := read(hotspotA, 2)
		if len(rows) != 1 || rows[0].HourBin != 0 {
			t.Errorf("got %+v, want only hour 0, the one hour with 2 or more checklists in the region", rows)
		}
	})
}

// TestCountryAndHotspotWeeks seeds hotspot rows for one week and checks the
// country table and the year of one species at one hotspot by hand. In week 10
// hotspot A (Lithuania) has 4 checklists, 2 with species X; hotspot B
// (Lithuania) has 1 with both X and Y; hotspot C (Latvia) has 2 with X. In week
// 11 A has 3 checklists, all with Y and none with X. Lithuania in week 10 is
// therefore 5 checklists, X on 3 of them (0.6) and Y on 1 (0.2); in week 11 it
// is 3 checklists, Y on all of them and X on none, so X has no row that week.
// Latvia's X in week 10 is 2 of 2.
func TestCountryAndHotspotWeeks(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newTestPool(t)
	queries := New(pool)

	seedHotspot := func(name, region string) int64 {
		t.Helper()
		row, err := queries.UpsertHotspot(ctx, UpsertHotspotParams{
			Name: name, Longitude: 25.0, Latitude: 55.0, Region: region, Source: "test", SourceRef: "test-weeks-" + name,
		})
		if err != nil {
			t.Fatalf("UpsertHotspot (%s): %v", name, err)
		}
		return row.ID
	}
	hotspotA := seedHotspot("a", "LT")
	hotspotB := seedHotspot("b", "LT")
	hotspotC := seedHotspot("c", "LV")
	emptyHotspot := seedHotspot("empty", "LT")

	seedSpecies := func(name, avibaseID string) int64 {
		t.Helper()
		row, err := queries.UpsertSpecies(ctx, UpsertSpeciesParams{ScientificName: name, AvibaseID: pgtype.Text{String: avibaseID, Valid: true}})
		if err != nil {
			t.Fatalf("UpsertSpecies (%s): %v", name, err)
		}
		return row.ID
	}
	speciesX := seedSpecies("Testus xenos", "avibase-TESTWKX1")
	speciesY := seedSpecies("Testus ypsilon", "avibase-TESTWKY1")

	seed := func(hotspotID, speciesID int64, week int, frequency float32, sampleSize int) {
		t.Helper()
		if _, err := pool.Exec(ctx,
			"INSERT INTO species_seasonality (hotspot_id, species_id, week_of_year, frequency, adjusted_frequency, sample_size) VALUES ($1, $2, $3, $4, $4, $5)",
			hotspotID, speciesID, week, frequency, sampleSize,
		); err != nil {
			t.Fatalf("seeding species_seasonality: %v", err)
		}
	}
	seed(hotspotA, speciesX, 10, 0.5, 4)
	seed(hotspotB, speciesX, 10, 1.0, 1)
	seed(hotspotB, speciesY, 10, 1.0, 1)
	seed(hotspotC, speciesX, 10, 1.0, 2)
	seed(hotspotA, speciesY, 11, 1.0, 3)

	rows, err := queries.RecomputeCountrySeasonality(ctx)
	if err != nil {
		t.Fatalf("RecomputeCountrySeasonality: %v", err)
	}
	if rows != 4 {
		t.Errorf("RecomputeCountrySeasonality wrote %d rows, want 4 (LT week 10 X and Y, LT week 11 Y, LV week 10 X)", rows)
	}

	weeks := func(hotspotID, speciesID int64) []GetHotspotSpeciesWeeksRow {
		t.Helper()
		got, err := queries.GetHotspotSpeciesWeeks(ctx, GetHotspotSpeciesWeeksParams{HotspotID: hotspotID, SpeciesID: speciesID})
		if err != nil {
			t.Fatalf("GetHotspotSpeciesWeeks: %v", err)
		}
		if len(got) != 52 {
			t.Fatalf("got %d weeks, want 52", len(got))
		}
		return got
	}
	check := func(got GetHotspotSpeciesWeeksRow, week int16, checklists, withSpecies int32, countryFrequency float64) {
		t.Helper()
		if got.WeekOfYear != week || got.Checklists != checklists || got.WithSpecies != withSpecies || math.Abs(float64(got.CountryFrequency)-countryFrequency) > 1e-6 {
			t.Errorf("week %d = %d of %d checklists, country %v; want week %d = %d of %d, country %v",
				got.WeekOfYear, got.WithSpecies, got.Checklists, got.CountryFrequency, week, withSpecies, checklists, countryFrequency)
		}
	}

	t.Run("a hotspot's year of a species", func(t *testing.T) {
		got := weeks(hotspotA, speciesX)
		check(got[0], 1, 0, 0, 0)
		check(got[9], 10, 4, 2, 0.6)
		// Week 11 has 3 checklists at A but none with X, and X has no country
		// row that week.
		check(got[10], 11, 3, 0, 0)
		check(got[11], 12, 0, 0, 0)
	})

	t.Run("a hotspot takes its own country's frequency", func(t *testing.T) {
		got := weeks(hotspotC, speciesX)
		check(got[9], 10, 2, 2, 1.0)
	})

	t.Run("a hotspot with no data is all zeros", func(t *testing.T) {
		got := weeks(emptyHotspot, speciesX)
		check(got[9], 10, 0, 0, 0.6)
	})
}

// TestSeasonalityCountsWeek53InWeek52 seeds a complete checklist in ISO week 52
// of 2026 and one in ISO week 53, which is 30 December, the second with the
// species. The year has 52 weeks, so both are week 52: 1 of 2 checklists.
func TestSeasonalityCountsWeek53InWeek52(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newTestPool(t)
	queries := New(pool)

	hotspot, err := queries.UpsertHotspot(ctx, UpsertHotspotParams{
		Name: "Week 53 hotspot", Longitude: 25.0, Latitude: 55.0, Region: "LT", Source: "test", SourceRef: "test-week-53",
	})
	if err != nil {
		t.Fatalf("UpsertHotspot: %v", err)
	}
	species, err := queries.UpsertSpecies(ctx, UpsertSpeciesParams{
		ScientificName: "Testus quinquaginta", AvibaseID: pgtype.Text{String: "avibase-TESTW531", Valid: true},
	})
	if err != nil {
		t.Fatalf("UpsertSpecies: %v", err)
	}

	seedChecklist := func(samplingEventID string, observedOn time.Time, withSpecies bool) {
		t.Helper()
		date := pgtype.Date{Time: observedOn, Valid: true}
		id, err := queries.UpsertChecklist(ctx, UpsertChecklistParams{
			SamplingEventID: samplingEventID, HotspotID: pgtype.Int8{Int64: hotspot.ID, Valid: true}, ObservedOn: date, AllSpeciesReported: true,
		})
		if err != nil {
			t.Fatalf("UpsertChecklist (%s): %v", samplingEventID, err)
		}
		if !withSpecies {
			return
		}
		if _, err := queries.UpsertObservation(ctx, UpsertObservationParams{
			SpeciesID: species.ID, HotspotID: pgtype.Int8{Int64: hotspot.ID, Valid: true}, ChecklistID: pgtype.Int8{Int64: id, Valid: true},
			Longitude: 25.0, Latitude: 55.0, ObservedOn: date, Source: "test", SourceRef: "obs-" + samplingEventID,
		}); err != nil {
			t.Fatalf("UpsertObservation (%s): %v", samplingEventID, err)
		}
	}
	seedChecklist("W52", time.Date(2026, 12, 24, 0, 0, 0, 0, time.UTC), false)
	seedChecklist("W53", time.Date(2026, 12, 30, 0, 0, 0, 0, time.UTC), true)

	if _, err := queries.RecomputeSpeciesSeasonality(ctx, 5); err != nil {
		t.Fatalf("RecomputeSpeciesSeasonality: %v", err)
	}

	assertSeasonality(ctx, t, pool, hotspot.ID, species.ID, 52, 0.5, 2, 0.5)
	var rowsInWeek53 int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM species_seasonality WHERE week_of_year = 53").Scan(&rowsInWeek53); err != nil {
		t.Fatalf("counting week 53 rows: %v", err)
	}
	if rowsInWeek53 != 0 {
		t.Errorf("%d rows in week 53, want none", rowsInWeek53)
	}
}

// TestAreaWeeks seeds hotspots around a point and checks which are within the
// range and the weeks pooled over them by hand. Near hotspot 1 has, in week 10,
// 4 checklists with species X on 2 and species Y on 1, and in week 11 2
// checklists, both with X. Near hotspot 2 has 1 checklist with X in week 10 and
// 2 with Y on 1 in week 12. The far hotspot, outside the range, has 100 in week
// 10, all with X, and must not count. Over the two near hotspots, for X: week 10
// is 3 of 5 checklists, week 11 is 2 of 2, week 12 is 0 of 2. For X and Y
// together week 10 is 4 (3 and 1) of 5, and week 12 is 1 of 2.
func TestAreaWeeks(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newTestPool(t)
	queries := New(pool)

	seedHotspot := func(name string, latitude, longitude float64) int64 {
		t.Helper()
		row, err := queries.UpsertHotspot(ctx, UpsertHotspotParams{
			Name: name, Latitude: latitude, Longitude: longitude, Region: "LT", Source: "test", SourceRef: "test-area-" + name,
		})
		if err != nil {
			t.Fatalf("UpsertHotspot (%s): %v", name, err)
		}
		return row.ID
	}
	near1 := seedHotspot("near1", 55.05, 25.05)
	near2 := seedHotspot("near2", 54.95, 24.95)
	far := seedHotspot("far", 57.0, 27.0)

	seedSpecies := func(name, avibaseID string) int64 {
		t.Helper()
		row, err := queries.UpsertSpecies(ctx, UpsertSpeciesParams{ScientificName: name, AvibaseID: pgtype.Text{String: avibaseID, Valid: true}})
		if err != nil {
			t.Fatalf("UpsertSpecies (%s): %v", name, err)
		}
		return row.ID
	}
	speciesX := seedSpecies("Testus areax", "avibase-TESTARX1")
	speciesY := seedSpecies("Testus areay", "avibase-TESTARY1")

	seed := func(hotspotID, speciesID int64, week int, frequency float32, sampleSize int) {
		t.Helper()
		if _, err := pool.Exec(ctx,
			"INSERT INTO species_seasonality (hotspot_id, species_id, week_of_year, frequency, adjusted_frequency, sample_size) VALUES ($1, $2, $3, $4, $4, $5)",
			hotspotID, speciesID, week, frequency, sampleSize,
		); err != nil {
			t.Fatalf("seeding species_seasonality: %v", err)
		}
	}
	seed(near1, speciesX, 10, 0.5, 4)
	seed(near1, speciesY, 10, 0.25, 4)
	seed(near1, speciesX, 11, 1.0, 2)
	seed(near2, speciesX, 10, 1.0, 1)
	seed(near2, speciesY, 12, 0.5, 2)
	seed(far, speciesX, 10, 1.0, 100)

	t.Run("finds the hotspots within the range", func(t *testing.T) {
		rows, err := queries.ListHotspotsWithin(ctx, ListHotspotsWithinParams{Latitude: 55.0, Longitude: 25.0, RadiusMeters: 25_000})
		if err != nil {
			t.Fatalf("ListHotspotsWithin: %v", err)
		}
		var ids []int64
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
		slices.Sort(ids)
		if want := []int64{near1, near2}; !slices.Equal(ids, want) {
			t.Fatalf("hotspots within = %v, want %v (the far one is outside 25 km)", ids, want)
		}
		if got := rows[0]; got.Latitude.Float64 < 54.9 || got.Latitude.Float64 > 55.1 || got.Longitude.Float64 < 24.9 || got.Longitude.Float64 > 25.1 {
			t.Errorf("coordinates = %v, %v, want near 55, 25", got.Latitude.Float64, got.Longitude.Float64)
		}
	})

	weeks := func(speciesIDs ...int64) []GetAreaWeeksRow {
		t.Helper()
		got, err := queries.GetAreaWeeks(ctx, GetAreaWeeksParams{HotspotIds: []int64{near1, near2}, SpeciesIds: speciesIDs})
		if err != nil {
			t.Fatalf("GetAreaWeeks: %v", err)
		}
		if len(got) != 52 {
			t.Fatalf("got %d weeks, want 52", len(got))
		}
		return got
	}
	check := func(got GetAreaWeeksRow, week int16, checklists, withSpecies int32) {
		t.Helper()
		if got.WeekOfYear != week || got.Checklists != checklists || got.WithSpecies != withSpecies {
			t.Errorf("week %d = %d of %d checklists; want week %d = %d of %d", got.WeekOfYear, got.WithSpecies, got.Checklists, week, withSpecies, checklists)
		}
	}

	t.Run("pools the hotspots for one species", func(t *testing.T) {
		got := weeks(speciesX)
		check(got[0], 1, 0, 0)
		check(got[9], 10, 5, 3)
		check(got[10], 11, 2, 2)
		check(got[11], 12, 2, 0)
	})

	t.Run("adds up several species", func(t *testing.T) {
		got := weeks(speciesX, speciesY)
		check(got[9], 10, 5, 4)
		check(got[11], 12, 2, 1)
	})
}

func assertSeasonality(ctx context.Context, t *testing.T, pool *pgxpool.Pool, hotspotID, speciesID int64, weekOfYear int, wantFrequency float64, wantSampleSize int32, wantAdjusted float64) {
	t.Helper()

	var frequency, adjusted float32
	var sampleSize int32
	err := pool.QueryRow(ctx,
		"SELECT frequency, adjusted_frequency, sample_size FROM species_seasonality WHERE hotspot_id = $1 AND species_id = $2 AND week_of_year = $3",
		hotspotID, speciesID, weekOfYear,
	).Scan(&frequency, &adjusted, &sampleSize)
	if err != nil {
		t.Fatalf("reading species_seasonality (hotspot=%d species=%d week=%d): %v", hotspotID, speciesID, weekOfYear, err)
	}
	// frequency is stored as a real (float32); dividing by 3 doesn't land on
	// an exact binary fraction, so this needs a coarser tolerance than
	// floatsClose's, which is calibrated for PostGIS coordinate round-trips.
	if math.Abs(float64(frequency)-wantFrequency) > 1e-4 {
		t.Errorf("frequency = %v, want %v", frequency, wantFrequency)
	}
	if math.Abs(float64(adjusted)-wantAdjusted) > 1e-4 {
		t.Errorf("adjusted_frequency = %v, want %v", adjusted, wantAdjusted)
	}
	if sampleSize != wantSampleSize {
		t.Errorf("sample_size = %d, want %d", sampleSize, wantSampleSize)
	}
}

func floatsClose(got, want float64) bool {
	return math.Abs(got-want) < 1e-9
}

// TestRecommendHotspots seeds three hotspots around one point -- two within
// a 25km search radius, one well outside it -- and checks that
// RecommendHotspots (a) excludes the far hotspot, (b) still lists a nearby
// hotspot with no seasonality data for the queried week (score 0, no
// species), (c) ranks species within a hotspot by adjusted frequency, not the
// raw one it returns, (d) respects
// TopSpeciesLimit, and (e) an explicit species_ids filter narrows both the
// species returned and the score computed from them. Species are ranked by
// adjusted frequency: the rare species has the higher raw frequency here but
// the lower adjusted one.
func TestRecommendHotspots(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newTestPool(t)
	queries := New(pool)

	const centerLat, centerLon = 55.0, 25.0
	const week = int16(10)

	near, err := queries.UpsertHotspot(ctx, UpsertHotspotParams{
		Name: "Near Hotspot", Latitude: 55.05, Longitude: 25.05,
		Region: "LT", Source: "test", SourceRef: "test-recommend-near",
	})
	if err != nil {
		t.Fatalf("UpsertHotspot (near): %v", err)
	}
	noData, err := queries.UpsertHotspot(ctx, UpsertHotspotParams{
		Name: "No Seasonality Data Hotspot", Latitude: 54.95, Longitude: 24.95,
		Region: "LT", Source: "test", SourceRef: "test-recommend-no-data",
	})
	if err != nil {
		t.Fatalf("UpsertHotspot (no data): %v", err)
	}
	_, err = queries.UpsertHotspot(ctx, UpsertHotspotParams{
		Name: "Far Hotspot", Latitude: 57.0, Longitude: 27.0,
		Region: "LT", Source: "test", SourceRef: "test-recommend-far",
	})
	if err != nil {
		t.Fatalf("UpsertHotspot (far): %v", err)
	}

	common, err := queries.UpsertSpecies(ctx, UpsertSpeciesParams{
		ScientificName: "Testus commonis",
		AvibaseID:      pgtype.Text{String: "avibase-TESTCMN1", Valid: true},
	})
	if err != nil {
		t.Fatalf("UpsertSpecies (common): %v", err)
	}
	rare, err := queries.UpsertSpecies(ctx, UpsertSpeciesParams{
		ScientificName: "Testus rarus",
		AvibaseID:      pgtype.Text{String: "avibase-TESTRAR1", Valid: true},
	})
	if err != nil {
		t.Fatalf("UpsertSpecies (rare): %v", err)
	}

	seedSeasonality := func(hotspotID, speciesID int64, frequency, adjustedFrequency float32) {
		t.Helper()
		_, err := pool.Exec(ctx,
			"INSERT INTO species_seasonality (hotspot_id, species_id, week_of_year, frequency, adjusted_frequency, sample_size) VALUES ($1, $2, $3, $4, $5, 10)",
			hotspotID, speciesID, week, frequency, adjustedFrequency,
		)
		if err != nil {
			t.Fatalf("seeding species_seasonality: %v", err)
		}
	}
	seedSeasonality(near.ID, common.ID, 0.8, 0.8)
	seedSeasonality(near.ID, rare.ID, 0.9, 0.3)
	// noData and the far hotspot deliberately get no seasonality rows: the
	// far one is proven excluded by radius alone, so it doesn't need any.

	recommend := func(params RecommendHotspotsParams) []RecommendHotspotsRow {
		t.Helper()
		params.Latitude = pgtype.Float8{Float64: centerLat, Valid: true}
		params.Longitude = pgtype.Float8{Float64: centerLon, Valid: true}
		params.RadiusMeters = pgtype.Float8{Float64: 25_000, Valid: true}
		params.WeekOfYear = pgtype.Int2{Int16: week, Valid: true}
		if params.TopSpeciesLimit.Int64 == 0 {
			params.TopSpeciesLimit = pgtype.Int8{Int64: 10, Valid: true}
		}
		rows, err := queries.RecommendHotspots(ctx, params)
		if err != nil {
			t.Fatalf("RecommendHotspots: %v", err)
		}
		return rows
	}

	t.Run("excludes the far hotspot and still lists the empty one", func(t *testing.T) {
		rows := recommend(RecommendHotspotsParams{})

		hotspots := map[int64]int{}
		for _, r := range rows {
			hotspots[r.HotspotID]++
		}
		if _, ok := hotspots[near.ID]; !ok {
			t.Errorf("near hotspot missing from results")
		}
		if _, ok := hotspots[noData.ID]; !ok {
			t.Errorf("no-data hotspot missing from results (should still be listed, just empty)")
		}
		if hotspots[near.ID] != 2 {
			t.Errorf("near hotspot returned %d rows, want 2 (one per species)", hotspots[near.ID])
		}
		if hotspots[noData.ID] != 1 {
			t.Errorf("no-data hotspot returned %d rows, want 1 (score-only row, no species)", hotspots[noData.ID])
		}
		for _, r := range rows {
			if r.HotspotID == noData.ID && (r.Score != 0 || r.SpeciesID.Valid) {
				t.Errorf("no-data hotspot row = %+v, want score=0 and no species", r)
			}
		}
	})

	t.Run("ranks species within a hotspot by adjusted frequency", func(t *testing.T) {
		rows := recommend(RecommendHotspotsParams{})

		var nearSpecies []RecommendHotspotsRow
		for _, r := range rows {
			if r.HotspotID == near.ID {
				nearSpecies = append(nearSpecies, r)
			}
		}
		if len(nearSpecies) != 2 || nearSpecies[0].SpeciesID.Int64 != common.ID || nearSpecies[1].SpeciesID.Int64 != rare.ID {
			t.Fatalf("near hotspot species order = %+v, want [common, rare]", nearSpecies)
		}
		if math.Abs(float64(nearSpecies[1].Frequency.Float32)-0.9) > 1e-4 || nearSpecies[1].Checklists.Int32 != 10 {
			t.Errorf("rare species frequency = %v of %d checklists, want the recorded 0.9 of 10", nearSpecies[1].Frequency.Float32, nearSpecies[1].Checklists.Int32)
		}
		// Score sums two float32 frequencies, so -- like assertSeasonality
		// above -- this needs a coarser tolerance than floatsClose's, which
		// is calibrated for float64 PostGIS coordinate round-trips.
		if math.Abs(float64(nearSpecies[0].Score)-1.1) > 1e-4 {
			t.Errorf("near hotspot score = %v, want ~1.1 (0.8 + 0.3, the adjusted frequencies)", nearSpecies[0].Score)
		}
	})

	t.Run("respects top_species_limit", func(t *testing.T) {
		rows := recommend(RecommendHotspotsParams{TopSpeciesLimit: pgtype.Int8{Int64: 1, Valid: true}})

		var nearSpecies int
		for _, r := range rows {
			if r.HotspotID == near.ID {
				nearSpecies++
			}
		}
		if nearSpecies != 1 {
			t.Errorf("near hotspot returned %d species rows, want 1 (top_species_limit=1)", nearSpecies)
		}
	})

	t.Run("an explicit species_ids filter narrows both species and score", func(t *testing.T) {
		rows := recommend(RecommendHotspotsParams{SpeciesIds: []int64{rare.ID}})

		for _, r := range rows {
			if r.HotspotID != near.ID {
				continue
			}
			if r.SpeciesID.Int64 != rare.ID {
				t.Errorf("species_ids filter let through species %d, want only %d", r.SpeciesID.Int64, rare.ID)
			}
			if math.Abs(float64(r.Score)-0.3) > 1e-4 {
				t.Errorf("filtered score = %v, want 0.3 (rare's adjusted frequency alone)", r.Score)
			}
		}
	})

	t.Run("an explicit species_ids filter leaves out hotspots with none of those species", func(t *testing.T) {
		for _, r := range recommend(RecommendHotspotsParams{SpeciesIds: []int64{rare.ID}}) {
			if r.HotspotID == noData.ID {
				t.Errorf("no-data hotspot listed for a species filter, want it left out: %+v", r)
			}
		}
	})
}

// TestSearchSpecies checks the picker query against real Postgres: a match in
// either language, the name returned in the requested language, scientific
// name matching, wildcard characters treated literally, and species with no
// hotspot observation left out.
func TestSearchSpecies(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newTestPool(t)
	queries := New(pool)

	hotspot, err := queries.UpsertHotspot(ctx, UpsertHotspotParams{
		Name: "Search Hotspot", Latitude: 55.0, Longitude: 25.0,
		Region: "LT", Source: "test", SourceRef: "test-search-hotspot",
	})
	if err != nil {
		t.Fatalf("UpsertHotspot: %v", err)
	}

	seedSpecies := func(scientificName, ebirdCode, ltName, enName string, observed bool) {
		t.Helper()
		species, err := queries.UpsertSpecies(ctx, UpsertSpeciesParams{
			ScientificName:   scientificName,
			EbirdSpeciesCode: pgtype.Text{String: ebirdCode, Valid: true},
			AvibaseID:        pgtype.Text{String: "avibase-" + ebirdCode, Valid: true},
		})
		if err != nil {
			t.Fatalf("UpsertSpecies (%s): %v", scientificName, err)
		}
		for lang, name := range map[string]string{"lt": ltName, "en": enName} {
			if _, err := queries.UpsertSpeciesName(ctx, UpsertSpeciesNameParams{
				EbirdSpeciesCode: pgtype.Text{String: ebirdCode, Valid: true},
				Lang:             lang, CommonName: name, Source: "test", IsPrimary: true,
			}); err != nil {
				t.Fatalf("UpsertSpeciesName (%s, %s): %v", ebirdCode, lang, err)
			}
		}
		if !observed {
			return
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO observations (species_id, hotspot_id, location, observed_on, source, source_ref)
			 VALUES ($1, $2, ST_SetSRID(ST_MakePoint(25, 55), 4326)::geography, '2026-05-01', 'test', $3)`,
			species.ID, hotspot.ID, "test-search-"+ebirdCode,
		); err != nil {
			t.Fatalf("seeding observation (%s): %v", ebirdCode, err)
		}
	}
	seedSpecies("Ciconia ciconia", "whitestk", "Baltasis gandras", "White Stork", true)
	seedSpecies("Ciconia nigra", "blackstk", "Juodasis gandras", "Black Stork", true)
	seedSpecies("Testus unseen", "unseenstk", "Nematytas gandras", "Unseen Stork", false)
	seedSpecies("Passer domesticus", "housesprw", "Naminis žvirblis", "House Sparrow", true)

	search := func(query string, langPriority ...string) []string {
		t.Helper()
		rows, err := queries.SearchSpecies(ctx, SearchSpeciesParams{Query: query, LangPriority: langPriority, ResultLimit: 10})
		if err != nil {
			t.Fatalf("SearchSpecies(%q): %v", query, err)
		}
		var names []string
		for _, row := range rows {
			names = append(names, row.CommonName)
		}
		return names
	}

	tests := []struct {
		name         string
		query        string
		langPriority []string
		want         []string
	}{
		{"matches the Lithuanian name, shown in Lithuanian", "gandras", []string{"lt", "en"}, []string{"Baltasis gandras", "Juodasis gandras"}},
		{"matches case-insensitively in English", "STORK", []string{"en", "lt"}, []string{"Black Stork", "White Stork"}},
		{"matches in one language, shown in the requested one", "gandras", []string{"en", "lt"}, []string{"Black Stork", "White Stork"}},
		{"matches the scientific name", "ciconia nigra", []string{"en", "lt"}, []string{"Black Stork"}},
		{"treats % as a plain character", "%", []string{"en", "lt"}, nil},
		{"finds a diacritic name typed without diacritics", "zvirbl", []string{"lt", "en"}, []string{"Naminis žvirblis"}},
		{"finds a plain name typed with diacritics", "gąndras", []string{"lt", "en"}, []string{"Baltasis gandras", "Juodasis gandras"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := search(tc.query, tc.langPriority...); !slices.Equal(got, tc.want) {
				t.Errorf("SearchSpecies(%q) = %v, want %v (the unobserved species must never appear)", tc.query, got, tc.want)
			}
		})
	}
}

// TestGetCommonNames seeds one species with both lt and en names and a
// second with only en, then checks that lang_priority actually drives which
// name wins -- this is what proves the DISTINCT ON + array_position query
// behaves as designed against real Postgres, not just as read on the page.
func TestGetCommonNames(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	queries := New(newTestPool(t))

	both, err := queries.UpsertSpecies(ctx, UpsertSpeciesParams{
		ScientificName:   "Ciconia ciconia",
		EbirdSpeciesCode: pgtype.Text{String: "whistk-test", Valid: true},
		AvibaseID:        pgtype.Text{String: "avibase-TESTBOTH1", Valid: true},
	})
	if err != nil {
		t.Fatalf("UpsertSpecies (both): %v", err)
	}
	enOnly, err := queries.UpsertSpecies(ctx, UpsertSpeciesParams{
		ScientificName:   "Testus englishonly",
		EbirdSpeciesCode: pgtype.Text{String: "engonly-test", Valid: true},
		AvibaseID:        pgtype.Text{String: "avibase-TESTENON1", Valid: true},
	})
	if err != nil {
		t.Fatalf("UpsertSpecies (en-only): %v", err)
	}

	seedName := func(ebirdCode, lang, commonName string) {
		t.Helper()
		if _, err := queries.UpsertSpeciesName(ctx, UpsertSpeciesNameParams{
			EbirdSpeciesCode: pgtype.Text{String: ebirdCode, Valid: true},
			Lang:             lang,
			CommonName:       commonName,
			Source:           "test",
			IsPrimary:        true,
		}); err != nil {
			t.Fatalf("UpsertSpeciesName (%s, %s): %v", ebirdCode, lang, err)
		}
	}
	seedName("whistk-test", "lt", "Baltasis gandras")
	seedName("whistk-test", "en", "White Stork")
	seedName("engonly-test", "en", "English Only Bird")

	speciesIDs := []int64{both.ID, enOnly.ID}

	ltFirst, err := queries.GetCommonNames(ctx, GetCommonNamesParams{SpeciesIds: speciesIDs, LangPriority: []string{"lt", "en"}})
	if err != nil {
		t.Fatalf("GetCommonNames (lt priority): %v", err)
	}
	names := make(map[int64]string, len(ltFirst))
	for _, row := range ltFirst {
		names[row.SpeciesID] = row.CommonName
	}
	if names[both.ID] != "Baltasis gandras" {
		t.Errorf("names[both] = %q, want %q (lt requested and available)", names[both.ID], "Baltasis gandras")
	}
	if names[enOnly.ID] != "English Only Bird" {
		t.Errorf("names[enOnly] = %q, want %q (falls back to en, no lt row)", names[enOnly.ID], "English Only Bird")
	}

	enFirst, err := queries.GetCommonNames(ctx, GetCommonNamesParams{SpeciesIds: speciesIDs, LangPriority: []string{"en", "lt"}})
	if err != nil {
		t.Fatalf("GetCommonNames (en priority): %v", err)
	}
	names = make(map[int64]string, len(enFirst))
	for _, row := range enFirst {
		names[row.SpeciesID] = row.CommonName
	}
	if names[both.ID] != "White Stork" {
		t.Errorf("names[both] = %q, want %q (en requested and available -- proves lang_priority actually changes the outcome)", names[both.ID], "White Stork")
	}
}
