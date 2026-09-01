package ebird

import (
	"context"
	"net/url"
)

// TaxonomyName is one species-rank entry from the eBird taxonomy, in one
// locale.
type TaxonomyName struct {
	SpeciesCode string
	CommonName  string
}

type taxonEntry struct {
	SpeciesCode string `json:"speciesCode"`
	ComName     string `json:"comName"`
	Category    string `json:"category"`
}

// Taxonomy fetches the eBird taxonomy in the given locale (e.g. "lt", "en")
// and returns the species-rank entries. eBird's taxonomy also lists
// subspecies, hybrids and other non-species ranks under "category"; those
// are filtered out here since species is the only rank our species table
// carries.
func (c *Client) Taxonomy(ctx context.Context, locale string) ([]TaxonomyName, error) {
	query := url.Values{"locale": {locale}, "fmt": {"json"}}

	var entries []taxonEntry
	if err := c.get(ctx, "/ref/taxonomy/ebird", query, &entries); err != nil {
		return nil, err
	}

	names := make([]TaxonomyName, 0, len(entries))
	for _, entry := range entries {
		if entry.Category != "species" {
			continue
		}
		names = append(names, TaxonomyName{SpeciesCode: entry.SpeciesCode, CommonName: entry.ComName})
	}
	return names, nil
}
