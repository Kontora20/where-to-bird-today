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

// preferredLangs is the common-name fallback order: Lithuanian first (the
// app's default locale), English second, and the species' scientific name
// if neither exists.
var preferredLangs = []string{"lt", "en"}

// Params is one recommendation request. SpeciesIDs is nil for "all species".
// Keeping it an explicit field -- rather than the query silently assuming
// every species -- means a future species-group filter (birds of prey,
// water birds) is just a new value here, not a rewrite of the ranking.
type Params struct {
	Latitude   float64
	Longitude  float64
	RadiusKM   float64
	Week       int16
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
}

// Store is the persistence this service needs, declared on the consuming
// side rather than in the store package. Go interfaces are satisfied
// implicitly, so *store.Queries already implements this without importing
// birding or knowing it exists.
type Store interface {
	RecommendHotspots(ctx context.Context, arg store.RecommendHotspotsParams) ([]store.RecommendHotspotsRow, error)
	GetPrimaryCommonNames(ctx context.Context, speciesIDs []int64) ([]store.GetPrimaryCommonNamesRow, error)
}

// Service ranks hotspots for a recommendation request.
type Service struct {
	store Store
}

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

	names, err := s.commonNames(ctx, speciesIDs)
	if err != nil {
		return nil, err
	}
	for i, r := range recommendations {
		for j, sp := range r.TopSpecies {
			recommendations[i].TopSpecies[j].CommonName = commonName(names, sp.SpeciesID, sp.ScientificName)
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
	var recommendations []Recommendation
	var speciesIDs []int64

	for _, row := range rows {
		if len(recommendations) == 0 || recommendations[len(recommendations)-1].HotspotID != row.HotspotID {
			recommendations = append(recommendations, Recommendation{
				HotspotID:   row.HotspotID,
				HotspotName: row.HotspotName,
				Latitude:    row.Latitude.Float64,
				Longitude:   row.Longitude.Float64,
				DistanceKM:  row.DistanceMeters.Float64 / 1000,
				Score:       float64(row.Score),
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
// species stays two round trips total, not N*topSpeciesPerHotspot+1.
func (s *Service) commonNames(ctx context.Context, speciesIDs []int64) (map[int64]map[string]string, error) {
	if len(speciesIDs) == 0 {
		return nil, nil
	}

	rows, err := s.store.GetPrimaryCommonNames(ctx, speciesIDs)
	if err != nil {
		return nil, fmt.Errorf("loading species names: %w", err)
	}

	names := make(map[int64]map[string]string, len(speciesIDs))
	for _, row := range rows {
		if names[row.SpeciesID] == nil {
			names[row.SpeciesID] = make(map[string]string, len(preferredLangs))
		}
		names[row.SpeciesID][row.Lang] = row.CommonName
	}
	return names, nil
}

// commonName picks the first available name in preferredLangs order,
// falling back to the scientific name if the species has no common name in
// any preferred language.
func commonName(names map[int64]map[string]string, speciesID int64, scientificName string) string {
	for _, lang := range preferredLangs {
		if name, ok := names[speciesID][lang]; ok {
			return name
		}
	}
	return scientificName
}
