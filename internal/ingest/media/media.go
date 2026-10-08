// Package media orchestrates species_media photo sourcing: run each
// configured provider in preference order and write whatever it finds.
package media

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Kontora20/where-to-bird-today/internal/httpsurl"
	"github.com/Kontora20/where-to-bird-today/internal/store"
)

// Source is the ingestion_runs.source value this job writes.
const Source = "species-media"

// Photo is one candidate image for a species, independent of which provider
// found it.
type Photo struct {
	SourceRef      string
	ImageURL       string
	ThumbURL       string
	Width          int
	Height         int
	License        string
	LicenseURL     string
	Attribution    string
	AttributionURL string
}

// Provider finds photos for a batch of species and reports the
// species_media.source value it writes under. Implementations may resolve
// every species in one bulk call or loop one at a time -- Sync only needs
// whatever was found, keyed by species_id.
type Provider interface {
	Name() string
	FindPhotos(ctx context.Context, species []store.Species) (map[int64][]Photo, error)
}

// Stats summarizes one Sync call, per provider in the order it ran.
type Stats struct {
	Providers []ProviderStats
}

// ProviderStats is one provider's contribution to a Sync run.
type ProviderStats struct {
	Provider      string
	SpeciesFound  int
	PhotosWritten int
}

// Sync runs each provider over species in order and upserts every photo it
// finds. rank increases per species across the whole run, so an earlier
// provider's photos always rank ahead of a later provider's, and each
// provider's own photo order for that species is preserved within that.
func Sync(ctx context.Context, queries *store.Queries, species []store.Species, providers []Provider) (Stats, error) {
	var stats Stats
	nextRank := make(map[int64]int32)

	for _, provider := range providers {
		found, err := provider.FindPhotos(ctx, species)
		if err != nil {
			return stats, fmt.Errorf("%s: %w", provider.Name(), err)
		}

		providerStats := ProviderStats{Provider: provider.Name(), SpeciesFound: len(found)}
		for speciesID, photos := range found {
			for _, photo := range photos {
				// A provider's links come from outside: a photo whose image is
				// not an https link is skipped, any other bad link is left out.
				photo.ImageURL = httpsurl.Normalize(photo.ImageURL)
				if photo.ImageURL == "" {
					continue
				}
				photo.ThumbURL = httpsurl.Normalize(photo.ThumbURL)
				photo.LicenseURL = httpsurl.Normalize(photo.LicenseURL)
				photo.AttributionURL = httpsurl.Normalize(photo.AttributionURL)
				err := queries.UpsertSpeciesMedia(ctx, store.UpsertSpeciesMediaParams{
					SpeciesID:      speciesID,
					Source:         provider.Name(),
					SourceRef:      photo.SourceRef,
					ImageUrl:       photo.ImageURL,
					ThumbUrl:       optionalText(photo.ThumbURL),
					Width:          optionalInt32(photo.Width),
					Height:         optionalInt32(photo.Height),
					License:        photo.License,
					LicenseUrl:     optionalText(photo.LicenseURL),
					Attribution:    photo.Attribution,
					AttributionUrl: optionalText(photo.AttributionURL),
					Rank:           nextRank[speciesID],
				})
				if err != nil {
					return stats, fmt.Errorf("%s: upserting photo for species %d: %w", provider.Name(), speciesID, err)
				}
				nextRank[speciesID]++
				providerStats.PhotosWritten++
			}
		}
		stats.Providers = append(stats.Providers, providerStats)
	}

	return stats, nil
}

func optionalText(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

func optionalInt32(n int) pgtype.Int4 {
	if n == 0 {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(n), Valid: true}
}
