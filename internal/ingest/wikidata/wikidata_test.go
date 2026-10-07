package wikidata

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Kontora20/where-to-bird-today/internal/store"
)

// sparqlBinding and commons fixtures below mirror the real shapes captured
// live from query.wikidata.org and commons.wikimedia.org while building this
// package, not invented examples.

func sparqlFixture(bindings ...[2]string) string {
	var b strings.Builder
	b.WriteString(`{"results":{"bindings":[`)
	for i, pair := range bindings {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"ebirdCode":{"value":%q},"image":{"value":%q}}`, pair[0], pair[1])
	}
	b.WriteString(`]}}`)
	return b.String()
}

func TestFindPhotosKeepsMultipleImagesPerSpecies(t *testing.T) {
	t.Parallel()

	// Real shape: White Stork (whisto1) carries 3 Commons images, Common
	// Starling (eursta) carries 2.
	sparqlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(sparqlFixture(
			[2]string{"whisto1", "http://commons.wikimedia.org/wiki/Special:FilePath/Ciconia%20ciconia%20-%2001.jpg"},
			[2]string{"whisto1", "http://commons.wikimedia.org/wiki/Special:FilePath/Ciconia%20ciconia.jpg"},
			[2]string{"whisto1", "http://commons.wikimedia.org/wiki/Special:FilePath/00%204439%20Storchennest.jpg"},
			[2]string{"eursta", "http://commons.wikimedia.org/wiki/Special:FilePath/Common%20Starling.jpg"},
			[2]string{"eursta", "http://commons.wikimedia.org/wiki/Special:FilePath/Toulouse%20Sturnus.jpg"},
		)))
	}))
	defer sparqlServer.Close()

	commonsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("iiurlwidth"); got != "120" {
			t.Errorf("iiurlwidth = %q, want the 120 px icon size", got)
		}
		titles := strings.Split(r.URL.Query().Get("titles"), "|")
		pages := make(map[string]any, len(titles))
		for i, title := range titles {
			if title == "File:00 4439 Storchennest.jpg" {
				// No LicenseShortName -- must be dropped, not stored.
				pages[fmt.Sprint(i)] = map[string]any{
					"title":     title,
					"imageinfo": []any{map[string]any{"url": "https://example.org/x.jpg", "extmetadata": map[string]any{}}},
				}
				continue
			}
			pages[fmt.Sprint(i)] = map[string]any{
				"title": title,
				"imageinfo": []any{map[string]any{
					"url":         "https://upload.wikimedia.org/" + title,
					"thumburl":    "https://thumb.wikimedia.org/" + title,
					"thumbwidth":  640,
					"thumbheight": 427,
					"extmetadata": map[string]any{
						"LicenseShortName": map[string]any{"value": "CC BY-SA 2.0"},
						"LicenseUrl":       map[string]any{"value": "https://creativecommons.org/licenses/by-sa/2.0"},
						"Artist":           map[string]any{"value": `<a rel="nofollow" href="https://flickr.com/people/x">marsupium photography</a>`},
					},
				}},
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"query": map[string]any{"pages": pages}})
	}))
	defer commonsServer.Close()

	client := NewClient(nil)
	client.sparqlURL = sparqlServer.URL
	client.commonsURL = commonsServer.URL

	species := []store.Species{
		{ID: 10, EbirdSpeciesCode: pgtype.Text{String: "whisto1", Valid: true}},
		{ID: 20, EbirdSpeciesCode: pgtype.Text{String: "eursta", Valid: true}},
	}

	found, err := client.FindPhotos(t.Context(), species)
	if err != nil {
		t.Fatalf("FindPhotos: %v", err)
	}

	// White Stork: 3 candidates in, 1 dropped for no licence -> 2 kept.
	if got := len(found[10]); got != 2 {
		t.Errorf("species 10 photos = %d, want 2 (one dropped for no licence)", got)
	}
	if got := len(found[20]); got != 2 {
		t.Errorf("species 20 photos = %d, want 2", got)
	}

	for _, photo := range found[10] {
		if photo.License != "CC BY-SA 2.0" {
			t.Errorf("photo license = %q, want %q", photo.License, "CC BY-SA 2.0")
		}
		if photo.Attribution != "marsupium photography" {
			t.Errorf("photo attribution = %q, want %q", photo.Attribution, "marsupium photography")
		}
		if photo.AttributionURL != "https://flickr.com/people/x" {
			t.Errorf("photo attribution URL = %q, want %q", photo.AttributionURL, "https://flickr.com/people/x")
		}
	}
}

func TestFetchCommonsInfoChunksAt50(t *testing.T) {
	t.Parallel()

	var requestSizes []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		titles := strings.Split(r.URL.Query().Get("titles"), "|")
		requestSizes = append(requestSizes, len(titles))
		_ = json.NewEncoder(w).Encode(map[string]any{"query": map[string]any{"pages": map[string]any{}}})
	}))
	defer server.Close()

	client := NewClient(nil)
	client.commonsURL = server.URL

	filenames := make([]string, 60)
	for i := range filenames {
		filenames[i] = fmt.Sprintf("File:test-%d.jpg", i)
	}

	if _, err := client.fetchCommonsInfo(t.Context(), filenames); err != nil {
		t.Fatalf("fetchCommonsInfo: %v", err)
	}

	want := []int{50, 10}
	if len(requestSizes) != len(want) || requestSizes[0] != want[0] || requestSizes[1] != want[1] {
		t.Errorf("request batch sizes = %v, want %v", requestSizes, want)
	}
}

func TestStripArtistHTML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		raw      string
		wantText string
		wantHref string
	}{
		{
			name:     "linked artist",
			raw:      `<a rel="nofollow" class="external text" href="https://www.flickr.com/people/43405950@N07">marsupium photography</a>`,
			wantText: "marsupium photography",
			wantHref: "https://www.flickr.com/people/43405950@N07",
		},
		{
			name:     "plain text, no link",
			raw:      "Jane Doe",
			wantText: "Jane Doe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotText, gotHref := stripArtistHTML(tt.raw)
			if gotText != tt.wantText {
				t.Errorf("text = %q, want %q", gotText, tt.wantText)
			}
			if gotHref != tt.wantHref {
				t.Errorf("href = %q, want %q", gotHref, tt.wantHref)
			}
		})
	}
}

func TestCommonsFilename(t *testing.T) {
	t.Parallel()

	got, err := commonsFilename("http://commons.wikimedia.org/wiki/Special:FilePath/2017.07.04.-01-Wendisch%20Rietz--Kranich.jpg")
	if err != nil {
		t.Fatalf("commonsFilename: %v", err)
	}
	want := "File:2017.07.04.-01-Wendisch Rietz--Kranich.jpg"
	if got != want {
		t.Errorf("commonsFilename = %q, want %q", got, want)
	}

	if _, err := commonsFilename("http://example.org/not-a-commons-url"); err == nil {
		t.Error("commonsFilename(unexpected shape) = nil error, want one")
	}
}
