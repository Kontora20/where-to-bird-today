// Package wikidata resolves species photos via Wikidata (eBird species code
// -> Commons filename) and Wikimedia Commons (filename -> licensed image
// URLs). The two are always used together as one pipeline, per ADR 0003:
// this is the primary, best-licensed photo source, tried before iNaturalist.
package wikidata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Kontora20/where-to-bird-today/internal/ingest/media"
	"github.com/Kontora20/where-to-bird-today/internal/store"
)

const (
	sparqlURL  = "https://query.wikidata.org/sparql"
	commonsURL = "https://commons.wikimedia.org/w/api.php"

	// maxImagesPerSpecies bounds how many Commons photos one species can
	// contribute. Wikidata's real counts run small (verified live: White
	// Stork carries 3, Common Starling 2), so this is a defensive cap, not
	// an expected limit.
	maxImagesPerSpecies = 5

	// commonsBatchSize is MediaWiki's titles-per-request cap for anonymous
	// callers (bots get more; we aren't one).
	commonsBatchSize = 50
)

// userAgent identifies this client per the Wikimedia User-Agent policy
// (https://meta.wikimedia.org/wiki/User-Agent_policy). Verified live: both
// query.wikidata.org and commons.wikimedia.org return 403 Forbidden for
// Go's default "Go-http-client" user agent and 200 for a descriptive one --
// this isn't optional politeness, requests fail without it.
const userAgent = "WhereToBirdToday/0.1 (https://github.com/Kontora20/where-to-bird-today)"

// Client resolves species photos via Wikidata and Wikimedia Commons.
type Client struct {
	httpClient *http.Client
	sparqlURL  string
	commonsURL string
}

// NewClient returns a Client. httpClient may be nil, in which case a client
// with a 30s timeout is used.
func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{httpClient: httpClient, sparqlURL: sparqlURL, commonsURL: commonsURL}
}

// Name is the species_media.source value this client writes.
func (c *Client) Name() string { return "wikidata" }

// candidate is one Wikidata-resolved Commons filename awaiting a licence
// lookup, still tied to the species it's a photo of.
type candidate struct {
	speciesID int64
	filename  string
}

// FindPhotos resolves photos for species with a non-empty EbirdSpeciesCode.
// Species with no eBird code (rare -- 99.9% of AviList has one) are simply
// absent from the result map, same as a species Wikidata has no photo for.
func (c *Client) FindPhotos(ctx context.Context, species []store.Species) (map[int64][]media.Photo, error) {
	codeToSpeciesID := make(map[string]int64, len(species))
	codes := make([]string, 0, len(species))
	for _, sp := range species {
		if !sp.EbirdSpeciesCode.Valid || sp.EbirdSpeciesCode.String == "" {
			continue
		}
		codeToSpeciesID[sp.EbirdSpeciesCode.String] = sp.ID
		codes = append(codes, sp.EbirdSpeciesCode.String)
	}
	if len(codes) == 0 {
		return map[int64][]media.Photo{}, nil
	}

	candidates, err := c.resolveCandidates(ctx, codes, codeToSpeciesID)
	if err != nil {
		return nil, fmt.Errorf("resolving Commons filenames: %w", err)
	}

	filenames := make([]string, 0, len(candidates))
	seen := make(map[string]bool, len(candidates))
	for _, cand := range candidates {
		if !seen[cand.filename] {
			seen[cand.filename] = true
			filenames = append(filenames, cand.filename)
		}
	}

	licensed, err := c.fetchCommonsInfo(ctx, filenames)
	if err != nil {
		return nil, fmt.Errorf("fetching Commons licence info: %w", err)
	}

	result := make(map[int64][]media.Photo)
	for _, cand := range candidates {
		if len(result[cand.speciesID]) >= maxImagesPerSpecies {
			continue
		}
		photo, ok := licensed[cand.filename]
		if !ok {
			continue // no imageinfo, or no licence -- not stored either way
		}
		result[cand.speciesID] = append(result[cand.speciesID], photo)
	}
	return result, nil
}

// sparqlResponse is the shape of a SPARQL JSON results document -- see
// https://www.w3.org/TR/sparql11-results-json/.
type sparqlResponse struct {
	Results struct {
		Bindings []struct {
			EbirdCode struct {
				Value string `json:"value"`
			} `json:"ebirdCode"`
			Image struct {
				Value string `json:"value"`
			} `json:"image"`
		} `json:"bindings"`
	} `json:"results"`
}

// resolveCandidates runs one SPARQL query for every code at once (Wikidata
// comfortably handles VALUES lists far larger than our species count) and
// turns each (ebirdCode, image) pair into a candidate. POST, not GET: a
// VALUES clause for 300+ codes risks exceeding a safe URL length, and POST
// is Wikidata's own documented approach for non-trivial queries.
func (c *Client) resolveCandidates(ctx context.Context, codes []string, codeToSpeciesID map[string]int64) ([]candidate, error) {
	query := buildSparqlQuery(codes)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.sparqlURL, strings.NewReader(url.Values{"query": {query}}.Encode()))
	if err != nil {
		return nil, fmt.Errorf("building SPARQL request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/sparql-results+json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requesting SPARQL query: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("SPARQL query: unexpected status %s", resp.Status)
	}

	var parsed sparqlResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decoding SPARQL response: %w", err)
	}

	var candidates []candidate
	for _, binding := range parsed.Results.Bindings {
		speciesID, ok := codeToSpeciesID[binding.EbirdCode.Value]
		if !ok {
			continue
		}
		filename, err := commonsFilename(binding.Image.Value)
		if err != nil {
			continue // an unexpected image URL shape; skip rather than fail the whole batch
		}
		candidates = append(candidates, candidate{speciesID: speciesID, filename: filename})
	}
	return candidates, nil
}

// buildSparqlQuery lists every code in a VALUES clause. Codes come from our
// own AviList-derived species table, not user input, so no escaping beyond
// quoting is needed.
func buildSparqlQuery(codes []string) string {
	var values strings.Builder
	for _, code := range codes {
		values.WriteString(`"`)
		values.WriteString(code)
		values.WriteString(`" `)
	}
	return fmt.Sprintf(
		`SELECT ?ebirdCode ?image WHERE { VALUES ?ebirdCode { %s} ?item wdt:P3444 ?ebirdCode. ?item wdt:P18 ?image. }`,
		values.String(),
	)
}

// commonsFilename extracts "File:Name.jpg" from a Wikidata P18 value, which
// is a Special:FilePath URL like
// ".../Special:FilePath/2017.07.04.-01-Wendisch%20Rietz--Kranich.jpg".
func commonsFilename(imageURL string) (string, error) {
	const marker = "Special:FilePath/"
	i := strings.Index(imageURL, marker)
	if i == -1 {
		return "", fmt.Errorf("unexpected Wikidata image URL: %s", imageURL)
	}
	name, err := url.PathUnescape(imageURL[i+len(marker):])
	if err != nil {
		return "", fmt.Errorf("decoding filename from %s: %w", imageURL, err)
	}
	return "File:" + name, nil
}

// commonsResponse is the shape of a MediaWiki imageinfo query response.
type commonsResponse struct {
	Query struct {
		Pages map[string]struct {
			Title     string `json:"title"`
			ImageInfo []struct {
				URL         string `json:"url"`
				ThumbURL    string `json:"thumburl"`
				ThumbWidth  int    `json:"thumbwidth"`
				ThumbHeight int    `json:"thumbheight"`
				ExtMetadata struct {
					LicenseShortName struct {
						Value string `json:"value"`
					} `json:"LicenseShortName"`
					LicenseURL struct {
						Value string `json:"value"`
					} `json:"LicenseUrl"`
					Artist struct {
						Value string `json:"value"`
					} `json:"Artist"`
				} `json:"extmetadata"`
			} `json:"imageinfo"`
		} `json:"pages"`
	} `json:"query"`
}

// fetchCommonsInfo looks up licence and URL info for filenames, batched at
// commonsBatchSize per request. A filename with no LicenseShortName is
// dropped -- species_media.license is NOT NULL, and an empty licence isn't
// a usable row per ADR 0003.
func (c *Client) fetchCommonsInfo(ctx context.Context, filenames []string) (map[string]media.Photo, error) {
	result := make(map[string]media.Photo, len(filenames))

	for start := 0; start < len(filenames); start += commonsBatchSize {
		end := min(start+commonsBatchSize, len(filenames))
		batch := filenames[start:end]

		reqURL := c.commonsURL + "?" + url.Values{
			"action":              {"query"},
			"prop":                {"imageinfo"},
			"iiprop":              {"url|extmetadata"},
			"iiurlwidth":          {"640"},
			"titles":              {strings.Join(batch, "|")},
			"iiextmetadatafilter": {"LicenseShortName|Artist|LicenseUrl"},
			"format":              {"json"},
		}.Encode()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return nil, fmt.Errorf("building Commons request: %w", err)
		}
		req.Header.Set("User-Agent", userAgent)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("requesting Commons imageinfo: %w", err)
		}

		var parsed commonsResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&parsed)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("commons imageinfo: unexpected status %s", resp.Status)
		}
		if decodeErr != nil {
			return nil, fmt.Errorf("decoding Commons response: %w", decodeErr)
		}

		for _, page := range parsed.Query.Pages {
			if len(page.ImageInfo) == 0 {
				continue // missing page, or no image info
			}
			info := page.ImageInfo[0]
			if info.ExtMetadata.LicenseShortName.Value == "" {
				continue
			}
			attribution, attributionURL := stripArtistHTML(info.ExtMetadata.Artist.Value)
			result[page.Title] = media.Photo{
				SourceRef:      page.Title,
				ImageURL:       info.URL,
				ThumbURL:       info.ThumbURL,
				Width:          info.ThumbWidth,
				Height:         info.ThumbHeight,
				License:        info.ExtMetadata.LicenseShortName.Value,
				LicenseURL:     info.ExtMetadata.LicenseURL.Value,
				Attribution:    attribution,
				AttributionURL: attributionURL,
			}
		}
	}
	return result, nil
}

var artistAnchorPattern = regexp.MustCompile(`<a[^>]*href="([^"]*)"[^>]*>([^<]*)</a>`)
var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)

// stripArtistHTML turns Commons' Artist field -- HTML, usually a single
// link, e.g. `<a href="...">name</a>` -- into plain attribution text and,
// where present, the linked URL.
//
// ponytail: this handles the one shape Commons actually returns in
// practice, not arbitrary HTML. Reach for golang.org/x/net/html if a future
// Artist value turns out to need real parsing (nested tags, multiple
// links).
func stripArtistHTML(raw string) (name, href string) {
	if match := artistAnchorPattern.FindStringSubmatch(raw); match != nil {
		return match[2], match[1]
	}
	return htmlTagPattern.ReplaceAllString(raw, ""), ""
}
