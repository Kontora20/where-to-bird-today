package ebird

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/Kontora20/where-to-bird-today/internal/store"
)

var testDay = time.Date(2026, time.October, 20, 0, 0, 0, 0, time.UTC)

func TestMain(m *testing.M) {
	requestGap = time.Millisecond
	os.Exit(m.Run())
}

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client, err := NewClient("test-key", server.Client())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client.baseURL = server.URL
	return client
}

func TestChecklistReadsTimeAndRecords(t *testing.T) {
	t.Parallel()

	var gotPath string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{
			"subId": "S1", "locId": "L1", "groupId": "G1", "obsDt": "2026-10-20 20:41",
			"obsTimeValid": true, "allObsReported": true,
			"obs": [
				{"speciesCode": "sonthr1", "obsId": "OBS10", "howManyStr": "2"},
				{"speciesCode": "comcra", "obsId": "OBS11", "howManyStr": "X"}
			]
		}`))
	})

	got, err := client.Checklist(context.Background(), "S1")
	if err != nil {
		t.Fatalf("Checklist: %v", err)
	}

	if gotPath != "/product/checklist/view/S1" {
		t.Errorf("path = %q, want /product/checklist/view/S1", gotPath)
	}
	if got.GroupID != "G1" || !got.AllSpeciesReported || !got.HasStartTime {
		t.Errorf("checklist = %+v, want group G1, complete, with a start time", got)
	}
	if got.ObservedAt.Hour() != 20 || got.ObservedAt.Minute() != 41 {
		t.Errorf("ObservedAt = %v, want 20:41", got.ObservedAt)
	}
	if len(got.Observations) != 2 || got.Observations[1].HowMany != "X" {
		t.Errorf("Observations = %+v, want two, the second counted X", got.Observations)
	}
}

func TestChecklistWithoutStartTime(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"subId": "S1", "locId": "L1", "obsDt": "2026-10-20", "obsTimeValid": false}`))
	})

	got, err := client.Checklist(context.Background(), "S1")
	if err != nil {
		t.Fatalf("Checklist: %v", err)
	}
	if got.HasStartTime {
		t.Error("HasStartTime = true for a checklist with no time, want false")
	}
}

func TestChecklistNotFound(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	if _, err := client.Checklist(context.Background(), "S1"); err == nil || !strings.Contains(err.Error(), ErrNotFound.Error()) {
		t.Fatalf("Checklist() error = %v, want ErrNotFound", err)
	}
}

func TestListRegionDayAsksCountiesWhenTheListIsFull(t *testing.T) {
	t.Parallel()

	var paths []string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/product/lists/EE/2026/10/20":
			full := make([]checklistRefEntry, maxListResults)
			for index := range full {
				full[index].SubID = "S" + strconv.Itoa(index)
			}
			_ = json.NewEncoder(w).Encode(full)
		case "/ref/region/list/subnational1/EE":
			_, _ = w.Write([]byte(`[{"code": "EE-37"}, {"code": "EE-39"}]`))
		default:
			_, _ = w.Write([]byte(`[{"subId": "S900", "locId": "L1"}]`))
		}
	})

	refs, err := listRegionDay(context.Background(), client, "EE", testDay)
	if err != nil {
		t.Fatalf("listRegionDay: %v", err)
	}

	if len(refs) != 2 {
		t.Errorf("got %d checklists, want the 2 the counties listed", len(refs))
	}
	want := []string{"/product/lists/EE/2026/10/20", "/ref/region/list/subnational1/EE", "/product/lists/EE-37/2026/10/20", "/product/lists/EE-39/2026/10/20"}
	if strings.Join(paths, " ") != strings.Join(want, " ") {
		t.Errorf("requests = %v, want %v", paths, want)
	}
}

func TestCallKeepsTheQuotaReserve(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("RateLimit", `"policy";r=`+strconv.Itoa(quotaReserve)+`,"burst";r=24`)
		_, _ = w.Write([]byte(`[]`))
	})

	if err := call(context.Background(), client, func() error {
		return client.get(context.Background(), "/x", nil, &[]string{})
	}); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if left, known := client.QuotaLeft(); !known || left != quotaReserve {
		t.Fatalf("QuotaLeft() = %d, %v, want %d, true", left, known, quotaReserve)
	}

	called := false
	err := call(context.Background(), client, func() error {
		called = true
		return nil
	})
	if !errors.Is(err, ErrRateLimited) || called {
		t.Errorf("second call = %v (request made: %v), want ErrRateLimited and no request", err, called)
	}
}

func newTestPool(t *testing.T) *pgxpool.Pool {
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

	return pool
}

func TestLoadChecklistsStoresHotspotChecklistsOnce(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newTestPool(t)
	queries := store.New(pool)

	if _, err := queries.UpsertHotspot(ctx, store.UpsertHotspotParams{
		Name: "Hotspot", Longitude: 25.3, Latitude: 54.6, Region: "LT", Source: "ebird", SourceRef: "L1",
	}); err != nil {
		t.Fatalf("UpsertHotspot: %v", err)
	}
	if _, err := queries.UpsertSpecies(ctx, store.UpsertSpeciesParams{
		ScientificName:   "Turdus philomelos",
		EbirdSpeciesCode: pgtype.Text{String: "sonthr1", Valid: true},
	}); err != nil {
		t.Fatalf("UpsertSpecies: %v", err)
	}

	var viewed []string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/product/lists/"):
			_, _ = w.Write([]byte(`[
				{"subId": "S1", "locId": "L1", "loc": {"latitude": 54.6, "longitude": 25.3}},
				{"subId": "S2", "locId": "L2", "loc": {"latitude": 54.7, "longitude": 25.4}}
			]`))
		case r.URL.Path == "/product/checklist/view/S1":
			viewed = append(viewed, "S1")
			_, _ = w.Write([]byte(`{
				"subId": "S1", "locId": "L1", "obsDt": "2026-10-20 07:30", "obsTimeValid": true, "allObsReported": true,
				"obs": [
					{"speciesCode": "sonthr1", "obsId": "OBS10", "howManyStr": "2"},
					{"speciesCode": "unknown1", "obsId": "OBS11", "howManyStr": "1"}
				]
			}`))
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	written, err := LoadChecklists(ctx, client, queries, []string{"LT"}, testDay)
	if err != nil {
		t.Fatalf("LoadChecklists: %v", err)
	}
	if written != 1 {
		t.Errorf("written = %d, want 1: only the checklist at the known hotspot", written)
	}

	latestLT, err := queries.GetLatestChecklistDate(ctx, "LT")
	if err != nil || !latestLT.Valid || !latestLT.Time.Equal(testDay) {
		t.Errorf("GetLatestChecklistDate(LT) = %v, %v, want %v", latestLT, err, testDay)
	}
	if latestLV, err := queries.GetLatestChecklistDate(ctx, "LV"); err != nil || latestLV.Valid {
		t.Errorf("GetLatestChecklistDate(LV) = %v, %v, want none", latestLV, err)
	}

	var observationRef string
	var observations int
	if err := pool.QueryRow(ctx, "SELECT count(*), min(source || ' ' || source_ref) FROM observations").Scan(&observations, &observationRef); err != nil {
		t.Fatalf("reading observations: %v", err)
	}
	if observations != 1 || observationRef != "ebd URN:CornellLabOfOrnithology:EBIRD:OBS10" {
		t.Errorf("observations = %d (%q), want only the species in species, under the EBD's source and identifier", observations, observationRef)
	}

	written, err = LoadChecklists(ctx, client, queries, []string{"LT"}, testDay)
	if err != nil {
		t.Fatalf("second LoadChecklists: %v", err)
	}
	if written != 0 || len(viewed) != 1 {
		t.Errorf("second run wrote %d and fetched %d checklists in all, want 0 and 1", written, len(viewed))
	}
}
