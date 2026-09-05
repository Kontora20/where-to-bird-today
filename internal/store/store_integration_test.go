package store

import (
	"context"
	"database/sql"
	"math"
	"testing"
	"time"

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

// TestRecomputeSpeciesSeasonality seeds one hotspot, two species, and five
// observations spread across one ISO week, then checks the recomputed
// frequency and sample_size by hand. Species A is seen on 3 of the week's 4
// active days (frequency 0.75), species B on 2 of them (0.5). It then
// removes one of those active days and recomputes again: a stale
// sample_size of 4 surviving that second run would mean the table wasn't
// actually truncated first, only appended to.
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

	speciesA, err := queries.UpsertSpecies(ctx, UpsertSpeciesParams{
		ScientificName: "Testus speciesa",
		AvibaseID:      pgtype.Text{String: "avibase-TESTAAA1", Valid: true},
	})
	if err != nil {
		t.Fatalf("UpsertSpecies (A): %v", err)
	}
	speciesB, err := queries.UpsertSpecies(ctx, UpsertSpeciesParams{
		ScientificName: "Testus speciesb",
		AvibaseID:      pgtype.Text{String: "avibase-TESTBBB1", Valid: true},
	})
	if err != nil {
		t.Fatalf("UpsertSpecies (B): %v", err)
	}

	seedObservation := func(sourceRef string, speciesID int64, day int) {
		t.Helper()
		_, err := queries.UpsertObservation(ctx, UpsertObservationParams{
			SpeciesID:  speciesID,
			HotspotID:  pgtype.Int8{Int64: hotspot.ID, Valid: true},
			Longitude:  25.0,
			Latitude:   55.0,
			ObservedOn: pgtype.Date{Time: time.Date(2026, 3, day, 0, 0, 0, 0, time.UTC), Valid: true},
			Source:     "test",
			SourceRef:  sourceRef,
		})
		if err != nil {
			t.Fatalf("seeding observation %s: %v", sourceRef, err)
		}
	}
	// All within the same ISO week (Mon 2026-03-02 to Fri 2026-03-06):
	// species A on the 2nd, 3rd and 4th; species B on the 2nd and 5th. Four
	// distinct days of activity at the hotspot in total.
	seedObservation("test-obs-a-1", speciesA.ID, 2)
	seedObservation("test-obs-a-2", speciesA.ID, 3)
	seedObservation("test-obs-a-3", speciesA.ID, 4)
	seedObservation("test-obs-b-1", speciesB.ID, 2)
	seedObservation("test-obs-b-2", speciesB.ID, 5)

	recompute := func() {
		t.Helper()
		if err := queries.TruncateSpeciesSeasonality(ctx); err != nil {
			t.Fatalf("TruncateSpeciesSeasonality: %v", err)
		}
		if _, err := queries.RecomputeSpeciesSeasonality(ctx); err != nil {
			t.Fatalf("RecomputeSpeciesSeasonality: %v", err)
		}
	}
	recompute()

	_, weekOfYear := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC).ISOWeek()

	assertSeasonality(ctx, t, pool, hotspot.ID, speciesA.ID, weekOfYear, 3.0/4.0, 4)
	assertSeasonality(ctx, t, pool, hotspot.ID, speciesB.ID, weekOfYear, 2.0/4.0, 4)

	if _, err := pool.Exec(ctx, "DELETE FROM observations WHERE source_ref = $1", "test-obs-b-2"); err != nil {
		t.Fatalf("removing observation: %v", err)
	}
	recompute()

	// Species A is now seen on all 3 remaining active days.
	assertSeasonality(ctx, t, pool, hotspot.ID, speciesA.ID, weekOfYear, 1.0, 3)
	assertSeasonality(ctx, t, pool, hotspot.ID, speciesB.ID, weekOfYear, 1.0/3.0, 3)
}

func assertSeasonality(ctx context.Context, t *testing.T, pool *pgxpool.Pool, hotspotID, speciesID int64, weekOfYear int, wantFrequency float64, wantSampleSize int32) {
	t.Helper()

	var frequency float32
	var sampleSize int32
	err := pool.QueryRow(ctx,
		"SELECT frequency, sample_size FROM species_seasonality WHERE hotspot_id = $1 AND species_id = $2 AND week_of_year = $3",
		hotspotID, speciesID, weekOfYear,
	).Scan(&frequency, &sampleSize)
	if err != nil {
		t.Fatalf("reading species_seasonality (hotspot=%d species=%d week=%d): %v", hotspotID, speciesID, weekOfYear, err)
	}
	// frequency is stored as a real (float32); dividing by 3 doesn't land on
	// an exact binary fraction, so this needs a coarser tolerance than
	// floatsClose's, which is calibrated for PostGIS coordinate round-trips.
	if math.Abs(float64(frequency)-wantFrequency) > 1e-4 {
		t.Errorf("frequency = %v, want %v", frequency, wantFrequency)
	}
	if sampleSize != wantSampleSize {
		t.Errorf("sample_size = %d, want %d", sampleSize, wantSampleSize)
	}
}

func floatsClose(got, want float64) bool {
	return math.Abs(got-want) < 1e-9
}
