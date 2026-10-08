package birding

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Kontora20/where-to-bird-today/internal/httpsurl"
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

// photoOf cleans the links again on the way out: rows stored before the
// ingest did so may hold a link that is not https.
func photoOf(medium store.SpeciesMedium) *Photo {
	return &Photo{
		ImageURL:       httpsurl.Normalize(medium.ImageUrl),
		ThumbURL:       httpsurl.Normalize(medium.ThumbUrl.String),
		License:        medium.License,
		LicenseURL:     httpsurl.Normalize(medium.LicenseUrl.String),
		Attribution:    medium.Attribution,
		AttributionURL: httpsurl.Normalize(medium.AttributionUrl.String),
	}
}

// LatestChecklist is the most recent eBird checklist at the hotspot that
// recorded the species: a link to the record behind the numbers.
type LatestChecklist struct {
	URL        string `json:"url"`
	ObservedOn string `json:"observed_on"`
}

// SpeciesDetail is everything the species detail panel shows (ADR 0004):
// name, outbound reference links, one photo, and this hotspot's
// week-by-week seasonality, its best weeks, and hour-by-hour time of day for
// the species.
// Content besides Photo is never missing -- a species with no photo yet
// still has a name and links.
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
	BestWeeks           []WeekRange      `json:"best_weeks"`
	TimeOfDay           []HourFrequency  `json:"time_of_day"`
	LatestChecklist     *LatestChecklist `json:"latest_checklist,omitempty"`
}

// panelStore is what SpeciesPanel needs from persistence: the species itself,
// and the three parts of the panel that have their own interfaces.
type panelStore interface {
	nameStore
	weekStore
	timeOfDayStore
	GetSpecies(ctx context.Context, id int64) (store.Species, error)
	ListSpeciesMedia(ctx context.Context, speciesID int64) ([]store.SpeciesMedium, error)
	GetLatestChecklistWithSpecies(ctx context.Context, arg store.GetLatestChecklistWithSpeciesParams) (store.GetLatestChecklistWithSpeciesRow, error)
}

// SpeciesPanel assembles the species detail panel.
type SpeciesPanel struct {
	store panelStore
}

func newSpeciesPanel(store panelStore) *SpeciesPanel {
	return &SpeciesPanel{store: store}
}

// SpeciesDetail returns the detail panel's contents for one species, scoped
// to how it behaves at one hotspot -- species_seasonality is measured per
// hotspot, so "seasonality" only means anything alongside a hotspot to
// measure it at. date sets the clock times the time of day is shown at; only
// its calendar date is used.
func (p *SpeciesPanel) SpeciesDetail(ctx context.Context, hotspotID, speciesID int64, lang string, date time.Time) (SpeciesDetail, error) {
	species, err := p.store.GetSpecies(ctx, speciesID)
	if err != nil {
		return SpeciesDetail{}, fmt.Errorf("loading species: %w", err)
	}

	names, err := commonNames(ctx, p.store, []int64{speciesID}, lang)
	if err != nil {
		return SpeciesDetail{}, err
	}
	commonName, ok := names[speciesID]
	if !ok {
		commonName = species.ScientificName
	}

	media, err := p.store.ListSpeciesMedia(ctx, speciesID)
	if err != nil {
		return SpeciesDetail{}, fmt.Errorf("loading species media: %w", err)
	}
	var photo *Photo
	if len(media) > 0 {
		// media is ordered by rank (ListSpeciesMedia's own ORDER BY), so the
		// first row is the best one, per ADR 0004's "one photo" in the panel.
		photo = photoOf(media[0])
	}

	weeks, bestWeeks, err := seasonality(ctx, p.store, hotspotID, speciesID, date)
	if err != nil {
		return SpeciesDetail{}, err
	}

	hours, err := timeOfDay(ctx, p.store, hotspotID, speciesID, date)
	if err != nil {
		return SpeciesDetail{}, err
	}

	var latestChecklist *LatestChecklist
	latest, err := p.store.GetLatestChecklistWithSpecies(ctx, store.GetLatestChecklistWithSpeciesParams{
		SpeciesID: speciesID,
		HotspotID: hotspotID,
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
		Seasonality:         weeks,
		BestWeeks:           bestWeeks,
		TimeOfDay:           hours,
		LatestChecklist:     latestChecklist,
	}, nil
}
