package store

import (
	"context"
	"database/sql"
	"math"
	"testing"

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

func floatsClose(got, want float64) bool {
	return math.Abs(got-want) < 1e-9
}
