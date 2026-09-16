// Package birding is the recommendation engine: turning a location, travel
// radius, and date into ranked hotspots with expected-species summaries.
package birding

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

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
	Latitude   float64
	Longitude  float64
	RadiusKM   float64
	Week       int16
	Lang       string
	SpeciesIDs []int64
}

// ExpectedSpecies is one species likely to be seen, ranked by frequency.
type ExpectedSpecies struct {
	SpeciesID      int64   `json:"species_id"`
	CommonName     string  `json:"common_name"`
	ScientificName string  `json:"scientific_name"`
	Frequency      float32 `json:"frequency"`
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

	// VerificationURL links back to the hotspot on the site it came from, so
	// nothing this read-only app says has to be taken on faith. Empty for a
	// source with no public page to point at.
	VerificationURL string `json:"verification_url,omitempty"`
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

// Store is the persistence this service needs, declared on the consuming
// side rather than in the store package. Go interfaces are satisfied
// implicitly, so *store.Queries already implements this without importing
// birding or knowing it exists.
type Store interface {
	RecommendHotspots(ctx context.Context, arg store.RecommendHotspotsParams) ([]store.RecommendHotspotsRow, error)
	GetCommonNames(ctx context.Context, arg store.GetCommonNamesParams) ([]store.GetCommonNamesRow, error)
	GetSpecies(ctx context.Context, id int64) (store.Species, error)
	ListSpeciesMedia(ctx context.Context, speciesID int64) ([]store.SpeciesMedium, error)
	GetSpeciesSeasonalityAtHotspot(ctx context.Context, arg store.GetSpeciesSeasonalityAtHotspotParams) ([]store.GetSpeciesSeasonalityAtHotspotRow, error)
}

// Service ranks hotspots for a recommendation request.
type Service struct {
	store Store
}

// NewService builds a Service backed by store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// Recommend returns hotspots within p.RadiusKM of the point, ranked by
// expected species richness for p.Week, each with its top expected species.
func (s *Service) Recommend(ctx context.Context, p Params) ([]Recommendation, error) {
	rows, err := s.store.RecommendHotspots(ctx, store.RecommendHotspotsParams{
		Latitude:        pgtype.Float8{Float64: p.Latitude, Valid: true},
		Longitude:       pgtype.Float8{Float64: p.Longitude, Valid: true},
		RadiusMeters:    pgtype.Float8{Float64: p.RadiusKM * 1000, Valid: true},
		WeekOfYear:      pgtype.Int2{Int16: p.Week, Valid: true},
		SpeciesIds:      p.SpeciesIDs,
		TopSpeciesLimit: pgtype.Int8{Int64: topSpeciesPerHotspot, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("ranking hotspots: %w", err)
	}

	recommendations, speciesIDs := groupByHotspot(rows)

	names, err := s.commonNames(ctx, speciesIDs, p.Lang)
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

// groupByHotspot rolls the flat SQL rows back up into one Recommendation per
// hotspot, and collects the distinct species IDs seen so the caller can
// resolve their names in one batched call. Rows for the same hotspot are
// guaranteed adjacent by the query's ORDER BY, so this is a single pass with
// no map keyed by hotspot ID.
func groupByHotspot(rows []store.RecommendHotspotsRow) ([]Recommendation, []int64) {
	// Not `var recommendations []Recommendation`: a nil slice marshals to
	// JSON `null`, and the Svelte app's `{#each recommendations}` throws on
	// null rather than treating it as empty.
	recommendations := []Recommendation{}
	var speciesIDs []int64

	for _, row := range rows {
		if len(recommendations) == 0 || recommendations[len(recommendations)-1].HotspotID != row.HotspotID {
			recommendations = append(recommendations, Recommendation{
				HotspotID:       row.HotspotID,
				HotspotName:     row.HotspotName,
				Latitude:        row.Latitude.Float64,
				Longitude:       row.Longitude.Float64,
				DistanceKM:      row.DistanceMeters.Float64 / 1000,
				Score:           float64(row.Score),
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
		})
		speciesIDs = append(speciesIDs, row.SpeciesID.Int64)
	}

	return recommendations, speciesIDs
}

// commonNames batches one lookup for every species across every hotspot,
// instead of one query per species -- N hotspots x topSpeciesPerHotspot
// species stays two round trips total, not N*topSpeciesPerHotspot+1. The
// fallback between lang and its alternate is resolved in SQL (ADR 0005),
// so the result already has at most one name per species -- no picking
// left to do here, unlike before this query existed.
func (s *Service) commonNames(ctx context.Context, speciesIDs []int64, lang string) (map[int64]string, error) {
	if len(speciesIDs) == 0 {
		return nil, nil
	}

	rows, err := s.store.GetCommonNames(ctx, store.GetCommonNamesParams{
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
