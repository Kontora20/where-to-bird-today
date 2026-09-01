package ebird

import (
	"context"
	"net/url"
)

// Hotspot is one eBird hotspot, in one region.
type Hotspot struct {
	LocID  string
	Name   string
	Region string
	Lat    float64
	Lng    float64
}

type hotspotEntry struct {
	LocID       string  `json:"locId"`
	LocName     string  `json:"locName"`
	CountryCode string  `json:"countryCode"`
	Lat         float64 `json:"lat"`
	Lng         float64 `json:"lng"`
}

// HotspotsByRegion fetches every hotspot eBird lists for regionCode (e.g.
// "LT").
func (c *Client) HotspotsByRegion(ctx context.Context, regionCode string) ([]Hotspot, error) {
	query := url.Values{"fmt": {"json"}}

	var entries []hotspotEntry
	if err := c.get(ctx, "/ref/hotspot/"+regionCode, query, &entries); err != nil {
		return nil, err
	}

	hotspots := make([]Hotspot, 0, len(entries))
	for _, entry := range entries {
		hotspots = append(hotspots, Hotspot{
			LocID:  entry.LocID,
			Name:   entry.LocName,
			Region: entry.CountryCode,
			Lat:    entry.Lat,
			Lng:    entry.Lng,
		})
	}
	return hotspots, nil
}
