// Package birding is the recommendation engine: turning a location, travel
// radius, and date into ranked hotspots with expected-species summaries.
package birding

import (
	"context"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Kontora20/where-to-bird-today/internal/routing"
	"github.com/Kontora20/where-to-bird-today/internal/store"
)

// topSpeciesPerHotspot caps how many expected species come back per hotspot.
// A plain constant: nothing today needs it tunable per request.
const topSpeciesPerHotspot = 10

// supportedLangs are the only two the app ships today (ADR 0005). otherOf
// keeps that fact in this one place rather than spreading a hardcoded pair
// through every caller.
var supportedLangs = []string{"lt", "en"}

func otherOf(lang string) string {
	if lang == supportedLangs[0] {
		return supportedLangs[1]
	}
	return supportedLangs[0]
}

// Params is one recommendation request. SpeciesIDs is nil for "all species".
// Keeping it an explicit field -- rather than the query silently assuming
// every species -- means a future species-group filter (birds of prey,
// water birds) is just a new value here, not a rewrite of the ranking.
// Lang is the single language resolved by the caller (content negotiation
// is an HTTP concern, decided in internal/api, not here).
type Params struct {
	Latitude  float64
	Longitude float64
	RadiusKM  float64
	// MaxMinutes, when above zero, limits results by drive time instead of
	// RadiusKM.
	MaxMinutes float64
	Week       int16
	// WeekRadius, when above zero, ranks by the week and that many weeks either
	// side pooled, so a hotspot with few checklists in one week has more to rest on.
	WeekRadius int
	Lang       string
	SpeciesIDs []int64
}

// ExpectedSpecies is one species likely to be seen. Frequency is the share of
// the hotspot's Checklists that recorded it, as eBird counts it; the species
// are ranked by an adjusted frequency that trusts a small Checklists less.
type ExpectedSpecies struct {
	SpeciesID      int64   `json:"species_id"`
	CommonName     string  `json:"common_name"`
	ScientificName string  `json:"scientific_name"`
	Frequency      float32 `json:"frequency"`
	Checklists     int32   `json:"checklists"`
}

// Recommendation is one ranked hotspot.
type Recommendation struct {
	HotspotID   int64             `json:"hotspot_id"`
	HotspotName string            `json:"hotspot_name"`
	Latitude    float64           `json:"latitude"`
	Longitude   float64           `json:"longitude"`
	DistanceKM  float64           `json:"distance_km"`
	Score       float64           `json:"score"`
	TopSpecies  []ExpectedSpecies `json:"top_species"`

	// DriveMinutes is the drive time from the search point, rounded up to
	// whole minutes. Zero (omitted) when the request used a radius.
	DriveMinutes int `json:"drive_minutes,omitempty"`

	// LastSeen says when each of the requested species was last recorded at
	// the hotspot, and links to the checklist. A species never recorded there
	// has no entry, and a request with no species has none at all.
	LastSeen []LastSeen `json:"last_seen,omitempty"`

	// VerificationURL links back to the hotspot on the site it came from, so
	// nothing this read-only app says has to be taken on faith. Empty for a
	// source with no public page to point at.
	VerificationURL string `json:"verification_url,omitempty"`
}

// LastSeen is the most recent record of one species at a hotspot.
type LastSeen struct {
	SpeciesID    int64  `json:"species_id"`
	ObservedOn   string `json:"observed_on"`
	ChecklistURL string `json:"checklist_url"`
}

// sourceEBird is the hotspots.source value eBird-sourced hotspots carry
// (see cmd/ingest-ebird-hotspots). It is the only source verificationURL
// knows how to link today.
const sourceEBird = "ebird"

// verificationURL turns a hotspot's (source, source_ref) into a link a
// birder can open to check the data against its origin. This app never
// submits anything upstream, so its only claim to trust is that every
// number traces back to the source that produced it -- sourceRef is
// eBird's locId for an eBird-sourced hotspot, kept as-is from ingestion for
// exactly this purpose.
func verificationURL(source, sourceRef string) string {
	switch source {
	case sourceEBird:
		return "https://ebird.org/hotspot/" + sourceRef
	default:
		return ""
	}
}

// nameStore looks up species names, which several parts of the engine show.
type nameStore interface {
	GetCommonNames(ctx context.Context, arg store.GetCommonNamesParams) ([]store.GetCommonNamesRow, error)
}

// recommendStore is what Recommender needs from persistence, declared on the
// consuming side rather than in the store package. Go interfaces are satisfied
// implicitly, so *store.Queries already implements it without importing
// birding or knowing it exists.
type recommendStore interface {
	nameStore
	RecommendHotspots(ctx context.Context, arg store.RecommendHotspotsParams) ([]store.RecommendHotspotsRow, error)
	GetLastSeen(ctx context.Context, arg store.GetLastSeenParams) ([]store.HotspotSpeciesLastSeen, error)
}

// Recommender ranks hotspots for a recommendation request. Its drive times may
// be nil, in which case requests limited by drive time fail with
// ErrTravelTimeUnavailable.
type Recommender struct {
	store recommendStore
	travel
}

func newRecommender(store recommendStore, driveTimes DriveTimeFinder) *Recommender {
	return &Recommender{store: store, travel: travel{driveTimes: driveTimes}}
}

// Recommend returns hotspots within p.RadiusKM of the point, or within
// p.MaxMinutes of driving when that is set, ranked by expected species
// richness for p.Week, each with its top expected species.
func (r *Recommender) Recommend(ctx context.Context, p Params) ([]Recommendation, error) {
	radiusKM, err := r.searchRadiusKM(p.RadiusKM, p.MaxMinutes)
	if err != nil {
		return nil, err
	}

	rows, err := r.store.RecommendHotspots(ctx, store.RecommendHotspotsParams{
		Latitude:        pgtype.Float8{Float64: p.Latitude, Valid: true},
		Longitude:       pgtype.Float8{Float64: p.Longitude, Valid: true},
		RadiusMeters:    pgtype.Float8{Float64: radiusKM * 1000, Valid: true},
		WeekOfYear:      pgtype.Int4{Int32: int32(p.Week), Valid: true},
		WeekRadius:      pgtype.Int4{Int32: int32(p.WeekRadius), Valid: true},
		PriorChecklists: pgtype.Float4{Float32: weekPriorChecklists, Valid: true},
		SpeciesIds:      p.SpeciesIDs,
		TopSpeciesLimit: pgtype.Int8{Int64: topSpeciesPerHotspot, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("ranking hotspots: %w", err)
	}

	recommendations := groupByHotspot(rows)

	if p.MaxMinutes > 0 {
		origin := routing.Point{Latitude: p.Latitude, Longitude: p.Longitude}
		recommendations, err = r.withinDriveTime(ctx, recommendations, origin, p.MaxMinutes)
		if err != nil {
			return nil, err
		}
	}

	if len(p.SpeciesIDs) > 0 {
		if err := r.attachLastSeen(ctx, recommendations, p.SpeciesIDs); err != nil {
			return nil, err
		}
	}

	names, err := commonNames(ctx, r.store, speciesIDs(recommendations), p.Lang)
	if err != nil {
		return nil, err
	}
	for i, r := range recommendations {
		for j, sp := range r.TopSpecies {
			name, ok := names[sp.SpeciesID]
			if !ok {
				name = sp.ScientificName
			}
			recommendations[i].TopSpecies[j].CommonName = name
		}
	}

	return recommendations, nil
}

// attachLastSeen fills in LastSeen for the requested species, in one query for
// all the hotspots that are left.
func (r *Recommender) attachLastSeen(ctx context.Context, recommendations []Recommendation, speciesIDs []int64) error {
	if len(recommendations) == 0 {
		return nil
	}
	hotspotIDs := make([]int64, len(recommendations))
	position := make(map[int64]int, len(recommendations))
	for i, recommendation := range recommendations {
		hotspotIDs[i] = recommendation.HotspotID
		position[recommendation.HotspotID] = i
	}

	rows, err := r.store.GetLastSeen(ctx, store.GetLastSeenParams{HotspotIds: hotspotIDs, SpeciesIds: speciesIDs})
	if err != nil {
		return fmt.Errorf("loading last seen: %w", err)
	}
	for _, row := range rows {
		i := position[row.HotspotID]
		recommendations[i].LastSeen = append(recommendations[i].LastSeen, LastSeen{
			SpeciesID:    row.SpeciesID,
			ObservedOn:   row.ObservedOn.Time.Format("2006-01-02"),
			ChecklistURL: "https://ebird.org/checklist/" + row.SamplingEventID,
		})
	}
	return nil
}

// withinDriveTime keeps the recommendations reachable from origin within
// maxMinutes of driving, in their existing order, and records each drive
// time. It returns an empty non-nil slice when none qualify.
func (r *Recommender) withinDriveTime(ctx context.Context, recommendations []Recommendation, origin routing.Point, maxMinutes float64) ([]Recommendation, error) {
	destinations := make([]routing.Point, len(recommendations))
	for i, recommendation := range recommendations {
		destinations[i] = routing.Point{Latitude: recommendation.Latitude, Longitude: recommendation.Longitude}
	}

	positions, times, err := r.reachable(ctx, origin, destinations, maxMinutes)
	if err != nil {
		return nil, err
	}

	reachable := make([]Recommendation, 0, len(positions))
	for i, position := range positions {
		recommendation := recommendations[position]
		recommendation.DriveMinutes = max(1, int(math.Ceil(times[i].Minutes())))
		reachable = append(reachable, recommendation)
	}
	return reachable, nil
}

// speciesIDs lists the species shown across recommendations, so the caller
// can resolve their names in one batched call.
func speciesIDs(recommendations []Recommendation) []int64 {
	var ids []int64
	for _, recommendation := range recommendations {
		for _, species := range recommendation.TopSpecies {
			ids = append(ids, species.SpeciesID)
		}
	}
	return ids
}

// groupByHotspot rolls the flat SQL rows back up into one Recommendation per
// hotspot. Rows for the same hotspot are guaranteed adjacent by the query's
// ORDER BY, so this is a single pass with no map keyed by hotspot ID.
func groupByHotspot(rows []store.RecommendHotspotsRow) []Recommendation {
	// Not `var recommendations []Recommendation`: a nil slice marshals to
	// JSON `null`, and the Svelte app's `{#each recommendations}` throws on
	// null rather than treating it as empty.
	recommendations := []Recommendation{}

	for _, row := range rows {
		if len(recommendations) == 0 || recommendations[len(recommendations)-1].HotspotID != row.HotspotID {
			recommendations = append(recommendations, Recommendation{
				HotspotID:   row.HotspotID,
				HotspotName: row.HotspotName,
				Latitude:    row.Latitude.Float64,
				Longitude:   row.Longitude.Float64,
				DistanceKM:  row.DistanceMeters.Float64 / 1000,
				Score:       float64(row.Score),
				// Same reasoning as the outer slice: a hotspot with no
				// seasonality rows this week must still send `[]`, not `null`.
				TopSpecies:      []ExpectedSpecies{},
				VerificationURL: verificationURL(row.HotspotSource, row.HotspotSourceRef),
			})
		}

		if !row.SpeciesID.Valid {
			// A hotspot with no seasonality rows for this week: still
			// listed (ST_DWithin found it), just with no expected species.
			continue
		}

		current := &recommendations[len(recommendations)-1]
		current.TopSpecies = append(current.TopSpecies, ExpectedSpecies{
			SpeciesID:      row.SpeciesID.Int64,
			ScientificName: row.ScientificName.String,
			Frequency:      row.Frequency.Float32,
			Checklists:     row.Checklists.Int32,
		})
	}

	return recommendations
}

// commonNames batches one lookup for every species across every hotspot,
// instead of one query per species -- N hotspots x topSpeciesPerHotspot
// species stays two round trips total, not N*topSpeciesPerHotspot+1. The
// fallback between lang and its alternate is resolved in SQL (ADR 0005),
// so the result already has at most one name per species -- no picking
// left to do here, unlike before this query existed.
func commonNames(ctx context.Context, lookup nameStore, speciesIDs []int64, lang string) (map[int64]string, error) {
	if len(speciesIDs) == 0 {
		return nil, nil
	}

	rows, err := lookup.GetCommonNames(ctx, store.GetCommonNamesParams{
		SpeciesIds:   speciesIDs,
		LangPriority: []string{lang, otherOf(lang)},
	})
	if err != nil {
		return nil, fmt.Errorf("loading species names: %w", err)
	}

	names := make(map[int64]string, len(rows))
	for _, row := range rows {
		names[row.SpeciesID] = row.CommonName
	}
	return names, nil
}
