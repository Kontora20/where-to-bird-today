package ebird

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Kontora20/where-to-bird-today/internal/ingest/ebd"
	"github.com/Kontora20/where-to-bird-today/internal/store"
)

// ChecklistsSource is the ingestion_runs.source value the checklist loader
// writes.
const ChecklistsSource = "ebird-checklists"

// maxListResults is the most checklists eBird lists for one region and day.
const maxListResults = 200

// requestGap is the wait before each request the checklist loader makes. It
// keeps the loader inside eBird's burst limit of 25 requests in 5 seconds. It
// is a variable so tests can shorten it.
var requestGap = time.Second

// quotaReserve is how much of eBird's hourly limit of 500 requests the
// loader leaves unused for the other jobs that share the API key. The loader
// stops when that many are left, and reports ErrRateLimited as if eBird had
// answered 429.
const quotaReserve = 50

// observationRefPrefix turns an eBird observation id into the identifier the
// EBD export gives the same record, so loading the export later updates these
// rows instead of adding a second copy.
const observationRefPrefix = "URN:CornellLabOfOrnithology:EBIRD:"

// ChecklistRef is one entry of a day's checklist list: the checklist and the
// place it was made.
type ChecklistRef struct {
	SubID     string
	LocID     string
	Latitude  float64
	Longitude float64
}

type checklistRefEntry struct {
	SubID string `json:"subId"`
	LocID string `json:"locId"`
	Loc   struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	} `json:"loc"`
}

// ChecklistRefs fetches the checklists made in regionCode on day, at most
// maxListResults of them.
func (c *Client) ChecklistRefs(ctx context.Context, regionCode string, day time.Time) ([]ChecklistRef, error) {
	path := fmt.Sprintf("/product/lists/%s/%d/%d/%d", regionCode, day.Year(), int(day.Month()), day.Day())
	query := url.Values{"maxResults": {strconv.Itoa(maxListResults)}}

	var entries []checklistRefEntry
	if err := c.get(ctx, path, query, &entries); err != nil {
		return nil, err
	}

	refs := make([]ChecklistRef, 0, len(entries))
	for _, entry := range entries {
		refs = append(refs, ChecklistRef{
			SubID:     entry.SubID,
			LocID:     entry.LocID,
			Latitude:  entry.Loc.Latitude,
			Longitude: entry.Loc.Longitude,
		})
	}
	return refs, nil
}

// SubregionCodes fetches the codes of the counties (or the like) of a country.
func (c *Client) SubregionCodes(ctx context.Context, countryCode string) ([]string, error) {
	var entries []struct {
		Code string `json:"code"`
	}
	if err := c.get(ctx, "/ref/region/list/subnational1/"+countryCode, nil, &entries); err != nil {
		return nil, err
	}

	codes := make([]string, 0, len(entries))
	for _, entry := range entries {
		codes = append(codes, entry.Code)
	}
	return codes, nil
}

// Checklist is one submitted checklist with its species records.
type Checklist struct {
	SubID              string
	LocID              string
	GroupID            string
	ObservedAt         time.Time
	HasStartTime       bool
	AllSpeciesReported bool
	Observations       []ChecklistObservation
}

// ChecklistObservation is one species record of a checklist. HowMany is the
// count as eBird shows it: a number, or "X" for present but not counted.
type ChecklistObservation struct {
	SpeciesCode string
	ObsID       string
	HowMany     string
}

type checklistEntry struct {
	SubID          string `json:"subId"`
	LocID          string `json:"locId"`
	GroupID        string `json:"groupId"`
	ObsDt          string `json:"obsDt"`
	ObsTimeValid   bool   `json:"obsTimeValid"`
	AllObsReported bool   `json:"allObsReported"`
	Obs            []struct {
		SpeciesCode string `json:"speciesCode"`
		ObsID       string `json:"obsId"`
		HowManyStr  string `json:"howManyStr"`
	} `json:"obs"`
}

// Checklist fetches one checklist with all its species records.
func (c *Client) Checklist(ctx context.Context, subID string) (Checklist, error) {
	var entry checklistEntry
	if err := c.get(ctx, "/product/checklist/view/"+subID, nil, &entry); err != nil {
		return Checklist{}, err
	}

	observedAt, err := parseObservedOn(entry.ObsDt)
	if err != nil {
		return Checklist{}, fmt.Errorf("checklist %s: %w", subID, err)
	}

	checklist := Checklist{
		SubID:              entry.SubID,
		LocID:              entry.LocID,
		GroupID:            entry.GroupID,
		ObservedAt:         observedAt,
		HasStartTime:       entry.ObsTimeValid && len(entry.ObsDt) > len(recentLayoutDateOnly),
		AllSpeciesReported: entry.AllObsReported,
	}
	for _, observation := range entry.Obs {
		checklist.Observations = append(checklist.Observations, ChecklistObservation{
			SpeciesCode: observation.SpeciesCode,
			ObsID:       observation.ObsID,
			HowMany:     observation.HowManyStr,
		})
	}
	return checklist, nil
}

// LoadChecklists fetches the checklists made on day in each of regions and
// stores those made at a known hotspot and not stored yet, with their records
// of species in species. It returns the number of checklists stored.
func LoadChecklists(ctx context.Context, client *Client, queries *store.Queries, regions []string, day time.Time) (int, error) {
	hotspotIDs, speciesIDs, err := loadIDs(ctx, queries)
	if err != nil {
		return 0, err
	}

	storedIDs, err := queries.ListChecklistSamplingEventIDsSince(ctx, pgtype.Date{Time: day, Valid: true})
	if err != nil {
		return 0, fmt.Errorf("listing stored checklists: %w", err)
	}
	stored := make(map[string]bool, len(storedIDs))
	for _, subID := range storedIDs {
		stored[subID] = true
	}

	refs := make(map[string]ChecklistRef)
	for _, region := range regions {
		listed, err := listRegionDay(ctx, client, region, day)
		if err != nil {
			return 0, fmt.Errorf("listing %s checklists of %s: %w", region, day.Format(time.DateOnly), err)
		}
		for _, ref := range listed {
			refs[ref.SubID] = ref
		}
	}

	written := 0
	for _, ref := range refs {
		hotspotID, knownHotspot := hotspotIDs[ref.LocID]
		if !knownHotspot || stored[ref.SubID] {
			continue
		}

		var checklist Checklist
		err := call(ctx, client, func() (err error) {
			checklist, err = client.Checklist(ctx, ref.SubID)
			return err
		})
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return written, fmt.Errorf("fetching checklist %s: %w", ref.SubID, err)
		}

		if err := storeChecklist(ctx, queries, checklist, ref, hotspotID, speciesIDs); err != nil {
			return written, err
		}
		written++
	}
	return written, nil
}

// listRegionDay lists a day's checklists of a region. When eBird fills the
// list, the region is asked again county by county so none are missed.
func listRegionDay(ctx context.Context, client *Client, region string, day time.Time) ([]ChecklistRef, error) {
	var refs []ChecklistRef
	err := call(ctx, client, func() (err error) {
		refs, err = client.ChecklistRefs(ctx, region, day)
		return err
	})
	if err != nil {
		return nil, err
	}
	if len(refs) < maxListResults {
		return refs, nil
	}
	if strings.Contains(region, "-") {
		slog.Warn("eBird checklist list is full, some checklists are missed", slog.String("region", region), slog.String("day", day.Format(time.DateOnly)))
		return refs, nil
	}

	var subregions []string
	err = call(ctx, client, func() (err error) {
		subregions, err = client.SubregionCodes(ctx, region)
		return err
	})
	if err != nil {
		return nil, err
	}
	var all []ChecklistRef
	for _, subregion := range subregions {
		listed, err := listRegionDay(ctx, client, subregion, day)
		if err != nil {
			return nil, err
		}
		all = append(all, listed...)
	}
	return all, nil
}

func storeChecklist(ctx context.Context, queries *store.Queries, checklist Checklist, ref ChecklistRef, hotspotID int64, speciesIDs map[string]int64) error {
	var startedAt pgtype.Time
	if checklist.HasStartTime {
		sinceMidnight := time.Duration(checklist.ObservedAt.Hour())*time.Hour + time.Duration(checklist.ObservedAt.Minute())*time.Minute
		startedAt = pgtype.Time{Microseconds: sinceMidnight.Microseconds(), Valid: true}
	}
	observedOn := pgtype.Date{Time: checklist.ObservedAt, Valid: true}

	checklistID, err := queries.UpsertChecklist(ctx, store.UpsertChecklistParams{
		SamplingEventID:    checklist.SubID,
		GroupID:            pgtype.Text{String: checklist.GroupID, Valid: checklist.GroupID != ""},
		HotspotID:          pgtype.Int8{Int64: hotspotID, Valid: true},
		ObservedOn:         observedOn,
		StartedAt:          startedAt,
		AllSpeciesReported: checklist.AllSpeciesReported,
	})
	if err != nil {
		return fmt.Errorf("storing checklist %s: %w", checklist.SubID, err)
	}

	for _, observation := range checklist.Observations {
		speciesID, knownSpecies := speciesIDs[observation.SpeciesCode]
		if !knownSpecies {
			continue
		}

		var count pgtype.Int4
		if parsed, err := strconv.Atoi(observation.HowMany); err == nil {
			count = pgtype.Int4{Int32: int32(parsed), Valid: true}
		}

		_, err := queries.UpsertObservation(ctx, store.UpsertObservationParams{
			SpeciesID:   speciesID,
			HotspotID:   pgtype.Int8{Int64: hotspotID, Valid: true},
			ChecklistID: pgtype.Int8{Int64: checklistID, Valid: true},
			Longitude:   ref.Longitude,
			Latitude:    ref.Latitude,
			ObservedOn:  observedOn,
			Count:       count,
			Source:      ebd.Source,
			SourceRef:   observationRefPrefix + observation.ObsID,
		})
		if err != nil {
			return fmt.Errorf("storing %s of checklist %s: %w", observation.SpeciesCode, checklist.SubID, err)
		}
	}
	return nil
}

// call runs one eBird request after waiting requestGap. It refuses with
// ErrRateLimited when no more than quotaReserve requests are left in the hour.
func call(ctx context.Context, client *Client, request func() error) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(requestGap):
	}
	if left, known := client.QuotaLeft(); known && left <= quotaReserve {
		return fmt.Errorf("%d requests left in the hour, keeping %d for other jobs: %w", left, quotaReserve, ErrRateLimited)
	}
	return request()
}
