package ebird

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClientRequiresAPIKey(t *testing.T) {
	t.Parallel()

	if _, err := NewClient("", nil); err == nil {
		t.Fatal("NewClient(\"\", nil) = nil error, want one")
	}
}

func TestTaxonomyFiltersToSpeciesRankAndSendsAuth(t *testing.T) {
	t.Parallel()

	var gotToken, gotLocale, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("X-eBirdApiToken")
		gotLocale = r.URL.Query().Get("locale")
		gotPath = r.URL.Path

		entries := []taxonEntry{
			{SpeciesCode: "comcra", ComName: "Common Crane", Category: "species"},
			{SpeciesCode: "comcra/x", ComName: "Common Crane/something", Category: "spuh"},
			{SpeciesCode: "whisto1", ComName: "White Stork", Category: "species"},
		}
		_ = json.NewEncoder(w).Encode(entries)
	}))
	defer server.Close()

	client, err := NewClient("test-key", server.Client())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client.baseURL = server.URL

	names, err := client.Taxonomy(context.Background(), "lt")
	if err != nil {
		t.Fatalf("Taxonomy: %v", err)
	}

	if gotToken != "test-key" {
		t.Errorf("X-eBirdApiToken = %q, want %q", gotToken, "test-key")
	}
	if gotLocale != "lt" {
		t.Errorf("locale query param = %q, want %q", gotLocale, "lt")
	}
	if gotPath != "/ref/taxonomy/ebird" {
		t.Errorf("path = %q, want %q", gotPath, "/ref/taxonomy/ebird")
	}

	want := []TaxonomyName{
		{SpeciesCode: "comcra", CommonName: "Common Crane"},
		{SpeciesCode: "whisto1", CommonName: "White Stork"},
	}
	if len(names) != len(want) {
		t.Fatalf("Taxonomy returned %d names, want %d: %+v", len(names), len(want), names)
	}
	for i, name := range names {
		if name != want[i] {
			t.Errorf("names[%d] = %+v, want %+v", i, name, want[i])
		}
	}
}

func TestHotspotsByRegion(t *testing.T) {
	t.Parallel()

	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path

		entries := []hotspotEntry{
			{LocID: "L14634126", LocName: "Alka Polder", CountryCode: "LT", Lat: 55.4382263, Lng: 21.3721427},
			{LocID: "L9055742", LocName: "Amalavo polderis", CountryCode: "LT", Lat: 54.5432936, Lng: 23.603611},
		}
		_ = json.NewEncoder(w).Encode(entries)
	}))
	defer server.Close()

	client, err := NewClient("test-key", server.Client())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client.baseURL = server.URL

	hotspots, err := client.HotspotsByRegion(context.Background(), "LT")
	if err != nil {
		t.Fatalf("HotspotsByRegion: %v", err)
	}

	if gotPath != "/ref/hotspot/LT" {
		t.Errorf("path = %q, want %q", gotPath, "/ref/hotspot/LT")
	}

	want := []Hotspot{
		{LocID: "L14634126", Name: "Alka Polder", Region: "LT", Lat: 55.4382263, Lng: 21.3721427},
		{LocID: "L9055742", Name: "Amalavo polderis", Region: "LT", Lat: 54.5432936, Lng: 23.603611},
	}
	if len(hotspots) != len(want) {
		t.Fatalf("HotspotsByRegion returned %d hotspots, want %d: %+v", len(hotspots), len(want), hotspots)
	}
	for i, hotspot := range hotspots {
		if hotspot != want[i] {
			t.Errorf("hotspots[%d] = %+v, want %+v", i, hotspot, want[i])
		}
	}
}

func TestRecentByRegion(t *testing.T) {
	t.Parallel()

	var gotPath, gotBack, gotHotspot string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotBack = r.URL.Query().Get("back")
		gotHotspot = r.URL.Query().Get("hotspot")

		entries := []recentEntry{
			{SpeciesCode: "comcra", LocID: "L1", SubID: "S1", ObsDt: "2026-09-24 07:30"},
			{SpeciesCode: "whisto1", LocID: "L2", SubID: "S2", ObsDt: "2026-09-25"},
		}
		_ = json.NewEncoder(w).Encode(entries)
	}))
	defer server.Close()

	client, err := NewClient("test-key", server.Client())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client.baseURL = server.URL

	got, err := client.RecentByRegion(context.Background(), "LT")
	if err != nil {
		t.Fatalf("RecentByRegion: %v", err)
	}

	if gotPath != "/data/obs/LT/recent" || gotBack != "7" || gotHotspot != "true" {
		t.Errorf("requested %s back=%s hotspot=%s, want /data/obs/LT/recent back=7 hotspot=true", gotPath, gotBack, gotHotspot)
	}
	if len(got) != 2 || got[0].ObservedOn.Format("2006-01-02") != "2026-09-24" || got[1].ObservedOn.Format("2006-01-02") != "2026-09-25" {
		t.Fatalf("RecentByRegion() = %+v, want two records dated 2026-09-24 and 2026-09-25", got)
	}
	if got[0].SpeciesCode != "comcra" || got[0].LocID != "L1" || got[0].SubID != "S1" {
		t.Errorf("first record = %+v, want comcra at L1 from S1", got[0])
	}
}

func TestRecentByRegionRejectsAnUnreadableDate(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]recentEntry{{SpeciesCode: "comcra", LocID: "L1", SubID: "S1", ObsDt: "yesterday"}})
	}))
	defer server.Close()

	client, err := NewClient("test-key", server.Client())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client.baseURL = server.URL

	if _, err := client.RecentByRegion(context.Background(), "LT"); err == nil {
		t.Fatal("RecentByRegion() = nil error for an unreadable date, want one")
	}
}

func TestTaxonomyUnexpectedStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client, err := NewClient("test-key", server.Client())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client.baseURL = server.URL

	if _, err := client.Taxonomy(context.Background(), "lt"); err == nil {
		t.Fatal("Taxonomy() = nil error on 401, want one")
	}
}
