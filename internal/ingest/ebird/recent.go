package ebird

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Kontora20/where-to-bird-today/internal/store"
)

// RecentSource is the ingestion_runs.source value the recent-observation
// loader writes.
const RecentSource = "ebird-recent"

// recentDays is how far back each run asks, eBird's maximum. A run that
// fails or is skipped for a few weeks is made up by the next one.
const recentDays = 30

// recentLayout is the format of obsDt: eBird drops the time when the
// observer gave none, so both forms occur.
const (
	recentLayoutWithTime = "2006-01-02 15:04"
	recentLayoutDateOnly = "2006-01-02"
)

// RecentObservation is the latest record of one species at one hotspot.
type RecentObservation struct {
	SpeciesCode string
	LocID       string
	SubID       string
	ObservedOn  time.Time
}

type recentEntry struct {
	SpeciesCode string `json:"speciesCode"`
	LocID       string `json:"locId"`
	SubID       string `json:"subId"`
	ObsDt       string `json:"obsDt"`
}

// RecentByRegion fetches the observations eBird lists for regionCode in the
// last recentDays days, at hotspots only. eBird answers with the latest
// record of each species at each location, not every record.
func (c *Client) RecentByRegion(ctx context.Context, regionCode string) ([]RecentObservation, error) {
	query := url.Values{
		"back":    {strconv.Itoa(recentDays)},
		"hotspot": {"true"},
	}

	var entries []recentEntry
	if err := c.get(ctx, "/data/obs/"+regionCode+"/recent", query, &entries); err != nil {
		return nil, err
	}

	observations := make([]RecentObservation, 0, len(entries))
	for _, entry := range entries {
		observedOn, err := parseObservedOn(entry.ObsDt)
		if err != nil {
			return nil, fmt.Errorf("record of %s at %s: %w", entry.SpeciesCode, entry.LocID, err)
		}
		observations = append(observations, RecentObservation{
			SpeciesCode: entry.SpeciesCode,
			LocID:       entry.LocID,
			SubID:       entry.SubID,
			ObservedOn:  observedOn,
		})
	}
	return observations, nil
}

func parseObservedOn(value string) (time.Time, error) {
	for _, layout := range []string{recentLayoutWithTime, recentLayoutDateOnly} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("obsDt %q is not a date", value)
}

// LoadRecent fetches the recent observations of each of regions and records
// each as the last sighting of its species at its hotspot when it is newer
// than the one stored (hotspot_species_last_seen). Records at locations that
// are not a known hotspot, or of a species with no eBird code in species,
// are skipped. It returns the number of rows written or moved forward.
func LoadRecent(ctx context.Context, client *Client, queries *store.Queries, regions []string) (int, error) {
	hotspotRefs, err := queries.ListHotspotRefsBySource(ctx, "ebird")
	if err != nil {
		return 0, fmt.Errorf("listing hotspots: %w", err)
	}
	hotspotIDs := make(map[string]int64, len(hotspotRefs))
	for _, ref := range hotspotRefs {
		hotspotIDs[ref.SourceRef] = ref.ID
	}

	speciesRows, err := queries.ListSpeciesEbirdCodes(ctx)
	if err != nil {
		return 0, fmt.Errorf("listing species codes: %w", err)
	}
	speciesIDs := make(map[string]int64, len(speciesRows))
	for _, row := range speciesRows {
		speciesIDs[row.EbirdSpeciesCode.String] = row.ID
	}

	written := 0
	for _, region := range regions {
		observations, err := client.RecentByRegion(ctx, region)
		if err != nil {
			return written, fmt.Errorf("fetching %s recent observations: %w", region, err)
		}

		for _, observation := range observations {
			hotspotID, knownHotspot := hotspotIDs[observation.LocID]
			speciesID, knownSpecies := speciesIDs[observation.SpeciesCode]
			if !knownHotspot || !knownSpecies {
				continue
			}

			rows, err := queries.UpsertLastSeen(ctx, store.UpsertLastSeenParams{
				HotspotID:       hotspotID,
				SpeciesID:       speciesID,
				ObservedOn:      pgtype.Date{Time: observation.ObservedOn, Valid: true},
				SamplingEventID: observation.SubID,
			})
			if err != nil {
				return written, fmt.Errorf("recording %s at %s: %w", observation.SpeciesCode, observation.LocID, err)
			}
			written += int(rows)
		}
	}
	return written, nil
}
