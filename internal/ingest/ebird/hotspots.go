package ebird

import (
	"context"
	"fmt"

	"github.com/Kontora20/where-to-bird-today/internal/store"
)

// HotspotSource is the ingestion_runs.source value the hotspot loader
// writes.
const HotspotSource = "ebird-hotspots"

// LoadHotspots fetches every hotspot eBird lists for each of regions and
// upserts it into hotspots. It returns the number of rows written.
func LoadHotspots(ctx context.Context, client *Client, queries *store.Queries, regions []string) (int, error) {
	count := 0
	for _, region := range regions {
		hotspots, err := client.HotspotsByRegion(ctx, region)
		if err != nil {
			return count, fmt.Errorf("fetching %s hotspots: %w", region, err)
		}

		for _, hotspot := range hotspots {
			_, err := queries.UpsertHotspot(ctx, store.UpsertHotspotParams{
				Name:      hotspot.Name,
				Longitude: hotspot.Lng,
				Latitude:  hotspot.Lat,
				Region:    hotspot.Region,
				Source:    "ebird",
				SourceRef: hotspot.LocID,
			})
			if err != nil {
				return count, fmt.Errorf("upserting hotspot %s: %w", hotspot.LocID, err)
			}
			count++
		}
	}
	return count, nil
}
