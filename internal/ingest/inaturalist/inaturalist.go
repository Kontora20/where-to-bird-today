// Package inaturalist is a client for the iNaturalist API, used as the
// gap-filling second photo source per ADR 0003: only Wikidata/Commons runs
// before it.
//
// The /v1/taxa search endpoint is a fuzzy text search across every kingdom,
// not an exact bird lookup -- verified live: searching a Common Crane's own
// scientific name "Grus grus" returns 119 results, and the top match is a
// Sandhill Crane synonym, with a puffball fungus (matched via an unrelated
// Polish common name) ranked second. The crane itself is third. The
// iconic_taxa=Aves request parameter does not filter this at all (checked:
// identical result count with or without it). Every lookup here filters
// client-side instead: iconic_taxon_name must be "Aves" and name must
// exactly equal the query, or it's treated as no photo, never a guess.
package inaturalist

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Kontora20/where-to-bird-today/internal/ingest/media"
	"github.com/Kontora20/where-to-bird-today/internal/store"
)

const (
	baseURL = "https://api.inaturalist.org/v1"

	// minRequestInterval paces every outgoing request. iNaturalist's
	// documented guidance: ~1 req/sec recommended, 100/min hard cap. A
	// plain delay is enough for a one-off batch job -- no token bucket, no
	// cache (see the eBird/iNat rate-limiting decision in project memory).
	minRequestInterval = time.Second

	// maxPhotosPerSpecies caps how many of a taxon's photos are stored, to
	// keep this in line with Wikidata's similarly small per-species count
	// rather than pulling in everything iNaturalist has.
	maxPhotosPerSpecies = 4
)

// userAgent identifies this client. iNaturalist doesn't enforce a policy
// like Wikimedia's (unlike query.wikidata.org and commons.wikimedia.org,
// verified live to 403 a generic Go client), but self-identifying to a
// third-party API is good practice regardless of current enforcement.
const userAgent = "WhereToBirdToday/0.1 (https://github.com/Kontora20/where-to-bird-today)"

// Client is a rate-limited iNaturalist API client.
type Client struct {
	httpClient    *http.Client
	baseURL       string
	lastRequestAt time.Time
}

// NewClient returns a Client. httpClient may be nil, in which case a client
// with a 30s timeout is used.
func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{httpClient: httpClient, baseURL: baseURL}
}

// Name is the species_media.source value this client writes.
func (c *Client) Name() string { return "inaturalist" }

// FindPhotos looks up each species by scientific name in turn, paced to
// stay well under iNaturalist's rate limit. A species with no exact,
// bird-ranked match is simply absent from the result -- this is the common
// case for the roughly 3% of species Wikidata already covers, and iNat
// still runs for every species per ADR 0003 ("supplies a second image").
func (c *Client) FindPhotos(ctx context.Context, species []store.Species) (map[int64][]media.Photo, error) {
	result := make(map[int64][]media.Photo)

	for _, sp := range species {
		taxonID, found, err := c.findTaxon(ctx, sp.ScientificName)
		if err != nil {
			return result, fmt.Errorf("searching for %s: %w", sp.ScientificName, err)
		}
		if !found {
			continue
		}

		photos, err := c.taxonPhotos(ctx, taxonID)
		if err != nil {
			return result, fmt.Errorf("fetching photos for %s: %w", sp.ScientificName, err)
		}
		if len(photos) > 0 {
			result[sp.ID] = photos
		}
	}
	return result, nil
}

// taxaSearchResult is the subset of a /v1/taxa search result this package
// needs -- a standalone named type so the exact-match filter is a pure,
// unit-testable function.
type taxaSearchResult struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	IconicTaxonName string `json:"iconic_taxon_name"`
}

type taxaSearchResponse struct {
	Results []taxaSearchResult `json:"results"`
}

func (c *Client) findTaxon(ctx context.Context, scientificName string) (int64, bool, error) {
	var parsed taxaSearchResponse
	query := url.Values{"q": {scientificName}, "rank": {"species"}}
	if err := c.get(ctx, "/taxa", query, &parsed); err != nil {
		return 0, false, err
	}
	taxonID, found := exactBirdMatch(parsed.Results, scientificName)
	return taxonID, found, nil
}

// exactBirdMatch is the safety filter this package exists for: a result set
// from a fuzzy cross-kingdom search is only usable if exactly one entry is
// both a bird and an exact name match.
func exactBirdMatch(results []taxaSearchResult, scientificName string) (int64, bool) {
	for _, result := range results {
		if result.IconicTaxonName == "Aves" && result.Name == scientificName {
			return result.ID, true
		}
	}
	return 0, false
}

// taxonPhoto is the subset of one /v1/taxa/{id} taxon_photos entry this
// package needs.
type taxonPhoto struct {
	ID                 int64  `json:"id"`
	LicenseCode        string `json:"license_code"`
	Attribution        string `json:"attribution"`
	SquareURL          string `json:"square_url"`
	MediumURL          string `json:"medium_url"`
	OriginalDimensions struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"original_dimensions"`
}

type taxonDetailResponse struct {
	Results []struct {
		TaxonPhotos []struct {
			Photo taxonPhoto `json:"photo"`
		} `json:"taxon_photos"`
	} `json:"results"`
}

func (c *Client) taxonPhotos(ctx context.Context, taxonID int64) ([]media.Photo, error) {
	var parsed taxonDetailResponse
	if err := c.get(ctx, "/taxa/"+strconv.FormatInt(taxonID, 10), nil, &parsed); err != nil {
		return nil, err
	}
	if len(parsed.Results) == 0 {
		return nil, nil
	}

	entries := make([]taxonPhoto, 0, len(parsed.Results[0].TaxonPhotos))
	for _, tp := range parsed.Results[0].TaxonPhotos {
		entries = append(entries, tp.Photo)
	}
	return photosFromTaxonPhotos(entries)
}

// photosFromTaxonPhotos converts taxon photos into media.Photo, dropping
// any with no licence_code -- species_media.license is NOT NULL, and
// iNaturalist does return null for some photos ("all rights reserved",
// verified live) -- and capping at maxPhotosPerSpecies.
func photosFromTaxonPhotos(entries []taxonPhoto) ([]media.Photo, error) {
	photos := make([]media.Photo, 0, min(len(entries), maxPhotosPerSpecies))
	for _, entry := range entries {
		if len(photos) >= maxPhotosPerSpecies {
			break
		}
		if entry.LicenseCode == "" {
			continue
		}
		photos = append(photos, media.Photo{
			SourceRef:   strconv.FormatInt(entry.ID, 10),
			ImageURL:    entry.MediumURL,
			ThumbURL:    entry.SquareURL,
			Width:       entry.OriginalDimensions.Width,
			Height:      entry.OriginalDimensions.Height,
			License:     entry.LicenseCode,
			Attribution: entry.Attribution,
		})
	}
	return photos, nil
}

// get issues a paced GET against path and decodes the JSON response into
// out. Every request goes through here so pacing can never be forgotten at
// a call site.
func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	c.pace()

	reqURL := c.baseURL + path
	if query != nil {
		reqURL += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("requesting %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("requesting %s: unexpected status %s", path, resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding %s response: %w", path, err)
	}
	return nil
}

// pace blocks until at least minRequestInterval has passed since the last
// request. Called from a single sequential loop (FindPhotos), so no
// synchronization is needed.
func (c *Client) pace() {
	if elapsed := time.Since(c.lastRequestAt); elapsed < minRequestInterval {
		time.Sleep(minRequestInterval - elapsed)
	}
	c.lastRequestAt = time.Now()
}
