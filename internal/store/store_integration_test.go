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
		if err := queries.TruncateSpeciesSeasonality(ctx); err != nil {
			t.Fatalf("TruncateSpeciesSeasonality: %v", err)
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
