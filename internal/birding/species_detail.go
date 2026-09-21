package birding

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Kontora20/where-to-bird-today/internal/store"
)

// Photo is one photo for a species, carrying the licence and attribution its
// source requires (ADR 0003) -- never the image bytes themselves.
type Photo struct {
	ImageURL       string `json:"image_url"`
	ThumbURL       string `json:"thumb_url,omitempty"`
	License        string `json:"license"`
	LicenseURL     string `json:"license_url,omitempty"`
	Attribution    string `json:"attribution"`
	AttributionURL string `json:"attribution_url,omitempty"`
}

// WeekFrequency is the share of complete checklists at one hotspot in one ISO
// week that recorded a species, and how many complete checklists that share
// is out of. A week missing from the list was never recorded, not recorded at
// zero frequency.
type WeekFrequency struct {
	Week       int16   `json:"week"`
	Frequency  float32 `json:"frequency"`
	Checklists int32   `json:"checklists"`
}

// LatestChecklist is the most recent eBird checklist at the hotspot that
// recorded the species: a link to the record behind the numbers.
type LatestChecklist struct {
	URL        string `json:"url"`
	ObservedOn string `json:"observed_on"`
}

// SpeciesDetail is everything the species detail panel shows (ADR 0004):
// name, outbound reference links, one photo, and this hotspot's
// week-by-week seasonality for the species. Content besides Photo is never
// missing -- a species with no photo yet still has a name and links.
//
// No Avibase link: not a data problem (species.avibase_id's truncated form
// does resolve on Avibase's site) but a value one -- its lang=LT page only
// translates the name, which is the one field eBird already gives us. See
// ADR 0004's addendum.
type SpeciesDetail struct {
	SpeciesID           int64            `json:"species_id"`
	CommonName          string           `json:"common_name"`
	ScientificName      string           `json:"scientific_name"`
	BirdsOfTheWorldURL  string           `json:"birds_of_the_world_url,omitempty"`
	BirdLifeDataZoneURL string           `json:"birdlife_datazone_url,omitempty"`
	EBirdURL            string           `json:"ebird_url,omitempty"`
	Photo               *Photo           `json:"photo,omitempty"`
	Seasonality         []WeekFrequency  `json:"seasonality"`
	LatestChecklist     *LatestChecklist `json:"latest_checklist,omitempty"`
}

// SpeciesDetail returns the detail panel's contents for one species, scoped
// to how it behaves at one hotspot -- species_seasonality is measured per
// hotspot, so "seasonality" only means anything alongside a hotspot to
// measure it at.
func (s *Service) SpeciesDetail(ctx context.Context, hotspotID, speciesID int64, lang string) (SpeciesDetail, error) {
	species, err := s.store.GetSpecies(ctx, speciesID)
	if err != nil {
		return SpeciesDetail{}, fmt.Errorf("loading species: %w", err)
	}

	names, err := s.commonNames(ctx, []int64{speciesID}, lang)
	if err != nil {
		return SpeciesDetail{}, err
	}
	commonName, ok := names[speciesID]
	if !ok {
		commonName = species.ScientificName
	}

	media, err := s.store.ListSpeciesMedia(ctx, speciesID)
	if err != nil {
		return SpeciesDetail{}, fmt.Errorf("loading species media: %w", err)
	}
	var photo *Photo
	if len(media) > 0 {
		// media is ordered by rank (ListSpeciesMedia's own ORDER BY), so the
		// first row is the best one, per ADR 0004's "one photo" in the panel.
		best := media[0]
		photo = &Photo{
			ImageURL:       best.ImageUrl,
			ThumbURL:       best.ThumbUrl.String,
			License:        best.License,
			LicenseURL:     best.LicenseUrl.String,
			Attribution:    best.Attribution,
			AttributionURL: best.AttributionUrl.String,
		}
	}

	seasonalityRows, err := s.store.GetSpeciesSeasonalityAtHotspot(ctx, store.GetSpeciesSeasonalityAtHotspotParams{
		HotspotID: hotspotID,
		SpeciesID: speciesID,
	})
	if err != nil {
		return SpeciesDetail{}, fmt.Errorf("loading species seasonality: %w", err)
	}
	seasonality := make([]WeekFrequency, len(seasonalityRows))
	for i, row := range seasonalityRows {
		seasonality[i] = WeekFrequency{Week: row.WeekOfYear, Frequency: row.Frequency, Checklists: row.SampleSize}
	}

	var latestChecklist *LatestChecklist
	latest, err := s.store.GetLatestChecklistWithSpecies(ctx, store.GetLatestChecklistWithSpeciesParams{
		SpeciesID: speciesID,
		HotspotID: pgtype.Int8{Int64: hotspotID, Valid: true},
	})
	switch {
	case err == nil:
		latestChecklist = &LatestChecklist{
			URL:        "https://ebird.org/checklist/" + latest.SamplingEventID,
			ObservedOn: latest.ObservedOn.Time.Format("2006-01-02"),
		}
	case !errors.Is(err, pgx.ErrNoRows):
		return SpeciesDetail{}, fmt.Errorf("loading latest checklist: %w", err)
	}

	var ebirdURL string
	if species.EbirdSpeciesCode.Valid {
		ebirdURL = "https://ebird.org/species/" + species.EbirdSpeciesCode.String
	}

	return SpeciesDetail{
		SpeciesID:           speciesID,
		CommonName:          commonName,
		ScientificName:      species.ScientificName,
		BirdsOfTheWorldURL:  species.BirdsOfTheWorldUrl.String,
		BirdLifeDataZoneURL: species.BirdlifeDatazoneUrl.String,
		EBirdURL:            ebirdURL,
		Photo:               photo,
		Seasonality:         seasonality,
		LatestChecklist:     latestChecklist,
	}, nil
}
