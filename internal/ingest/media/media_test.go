package media

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/Kontora20/where-to-bird-today/internal/store"
)

// fakeProvider is a MediaProvider stand-in: no network, no rate limiting,
// just the canned result Sync's orchestration logic is actually being
// tested against.
type fakeProvider struct {
	name   string
	photos map[int64][]Photo
}

func (f fakeProvider) Name() string { return f.name }

func (f fakeProvider) FindPhotos(_ context.Context, _ []store.Species) (map[int64][]Photo, error) {
	return f.photos, nil
}

// newTestQueries starts a real Postgres+PostGIS container migrated to the
// current schema -- Sync writes through UpsertSpeciesMedia, a real SQL
// statement worth exercising for real, same reasoning as
// internal/store's own integration tests.
func newTestQueries(t *testing.T) *store.Queries {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping Postgres-backed integration test in -short mode")
	}

	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx, "postgis/postgis:18-3.6",
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

	// A Provider holds its own state. goose.SetDialect and goose.Up share a
	// package-level variable, which tests running in parallel would race on.
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, os.DirFS("../../../migrations"))
	if err != nil {
		t.Fatalf("creating migration provider: %v", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("running migrations: %v", err)
	}

	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		t.Fatalf("opening pgx pool: %v", err)
	}
	t.Cleanup(pool.Close)

	return store.New(pool)
}

func seedSpecies(ctx context.Context, t *testing.T, queries *store.Queries, scientificName, avibaseID string) store.Species {
	t.Helper()
	sp, err := queries.UpsertSpecies(ctx, store.UpsertSpeciesParams{
		ScientificName: scientificName,
		AvibaseID:      pgtype.Text{String: avibaseID, Valid: true},
	})
	if err != nil {
		t.Fatalf("UpsertSpecies(%s): %v", scientificName, err)
	}
	return sp
}

func TestSyncOrdersRankAcrossProviders(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	queries := newTestQueries(t)

	crane := seedSpecies(ctx, t, queries, "Grus grus", "avibase-99B8841E")
	stork := seedSpecies(ctx, t, queries, "Ciconia ciconia", "avibase-28825494")

	wikidata := fakeProvider{
		name: "wikidata",
		photos: map[int64][]Photo{
			crane.ID: {{SourceRef: "wd-1", ImageURL: "https://example.org/wd-1.jpg", License: "CC0"}},
		},
	}
	inat := fakeProvider{
		name: "inaturalist",
		photos: map[int64][]Photo{
			crane.ID: {{SourceRef: "inat-1", ImageURL: "https://example.org/inat-1.jpg", License: "cc-by-nc"}},
			stork.ID: {{SourceRef: "inat-2", ImageURL: "https://example.org/inat-2.jpg", License: "cc-by-nc"}},
		},
	}

	stats, err := Sync(ctx, queries, []store.Species{crane, stork}, []Provider{wikidata, inat})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if len(stats.Providers) != 2 {
		t.Fatalf("Stats.Providers has %d entries, want 2", len(stats.Providers))
	}
	if stats.Providers[0].Provider != "wikidata" || stats.Providers[0].PhotosWritten != 1 {
		t.Errorf("wikidata stats = %+v, want 1 photo written", stats.Providers[0])
	}
	if stats.Providers[1].Provider != "inaturalist" || stats.Providers[1].PhotosWritten != 2 {
		t.Errorf("inaturalist stats = %+v, want 2 photos written", stats.Providers[1])
	}

	// The crane has one photo from each provider: wikidata ran first, so
	// its photo must rank ahead of inaturalist's for the SAME species.
	rows, err := queries.ListSpeciesMedia(ctx, crane.ID)
	if err != nil {
		t.Fatalf("ListSpeciesMedia: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("crane has %d species_media rows, want 2", len(rows))
	}
	byRank := map[int32]string{}
	for _, row := range rows {
		byRank[row.Rank] = row.Source
	}
	if byRank[0] != "wikidata" {
		t.Errorf("rank 0 source = %q, want %q", byRank[0], "wikidata")
	}
	if byRank[1] != "inaturalist" {
		t.Errorf("rank 1 source = %q, want %q", byRank[1], "inaturalist")
	}

	// The stork only has an inaturalist photo (wikidata found nothing for
	// it) -- it must still land at rank 0, not skipped or numbered as if
	// wikidata had written something first.
	storkRows, err := queries.ListSpeciesMedia(ctx, stork.ID)
	if err != nil {
		t.Fatalf("ListSpeciesMedia (stork): %v", err)
	}
	if len(storkRows) != 1 || storkRows[0].Rank != 0 {
		t.Errorf("stork rows = %+v, want exactly one row at rank 0", storkRows)
	}
}

func TestSyncKeepsOnlyHTTPSLinks(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	queries := newTestQueries(t)

	crane := seedSpecies(ctx, t, queries, "Grus grus", "avibase-99B8841E")

	provider := fakeProvider{
		name: "wikidata",
		photos: map[int64][]Photo{
			crane.ID: {
				{
					SourceRef:      "clean",
					ImageURL:       "https://example.org/clean.jpg",
					ThumbURL:       "data:text/html,<script>alert(1)</script>",
					License:        "CC BY-SA 3.0",
					LicenseURL:     "http://creativecommons.org/licenses/by-sa/3.0/",
					AttributionURL: "javascript:alert(1)",
				},
				{SourceRef: "bad-image", ImageURL: "javascript:alert(1)", License: "CC0"},
			},
		},
	}

	stats, err := Sync(ctx, queries, []store.Species{crane}, []Provider{provider})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if got := stats.Providers[0].PhotosWritten; got != 1 {
		t.Errorf("PhotosWritten = %d, want 1 (the photo with a bad image link is skipped)", got)
	}

	rows, err := queries.ListSpeciesMedia(ctx, crane.ID)
	if err != nil {
		t.Fatalf("ListSpeciesMedia: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("crane has %d species_media rows, want 1", len(rows))
	}
	row := rows[0]
	if row.ThumbUrl.Valid {
		t.Errorf("ThumbUrl = %q, want none", row.ThumbUrl.String)
	}
	if row.AttributionUrl.Valid {
		t.Errorf("AttributionUrl = %q, want none", row.AttributionUrl.String)
	}
	if got, want := row.LicenseUrl.String, "https://creativecommons.org/licenses/by-sa/3.0/"; got != want {
		t.Errorf("LicenseUrl = %q, want %q (http is upgraded)", got, want)
	}
}
