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

func TestTaxonomyUnexpectedStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
