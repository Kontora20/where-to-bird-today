package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"golang.org/x/text/language"

	"github.com/Kontora20/where-to-bird-today/internal/birding"
)

// dateParamLayout matches the plain "YYYY-MM-DD" a birder would type, not
// RFC 3339. This isn't a claim that time-of-day never matters for birding --
// it's that nothing downstream could use one yet: species_seasonality
// aggregates by week_of_year only. Accepting a timestamp today would mean
// silently discarding the time half of it.
const dateParamLayout = "2006-01-02"

// supportedLanguages is Lithuanian-first: NewMatcher uses the first tag as
// its own fallback (ADR 0005 -- "Lithuanian is the default in every
// ambiguous case"), so that's a property of this construction, not a
// special case written by hand anywhere below.
var supportedLanguages = language.NewMatcher([]language.Tag{language.Lithuanian, language.English})

// languageCodes is index-aligned with supportedLanguages' tag list, so a
// Matcher's returned index can be turned back into the plain "lt"/"en" the
// rest of the app (and the database's lang column) uses.
var languageCodes = []string{"lt", "en"}

// resolveLanguage picks the response language for r: an explicit "?lang="
// override if present and understood, else negotiated from Accept-Language,
// else Lithuanian -- the Matcher's own configured default when neither
// candidate parses or matches. MatchStrings tries each string in turn and
// falls through silently on anything it can't parse, so an absent or
// malformed value in either source never needs a special-cased branch here.
func resolveLanguage(r *http.Request) string {
	_, index := language.MatchStrings(supportedLanguages, r.URL.Query().Get("lang"), r.Header.Get("Accept-Language"))
	return languageCodes[index]
}

// maxTargetSpecies equals the number of species a hotspot card lists, so
// every chosen target can appear on it.
const maxTargetSpecies = 10

// maxDriveMinutes bounds ?max_minutes=. Four hours already reaches every
// hotspot in the Baltics from anywhere in them.
const maxDriveMinutes = 240

type recommendationService interface {
	Recommend(ctx context.Context, params birding.Params) ([]birding.Recommendation, error)
	SpeciesDetail(ctx context.Context, hotspotID, speciesID int64, lang string) (birding.SpeciesDetail, error)
	SearchSpecies(ctx context.Context, query, lang string) ([]birding.SpeciesMatch, error)
}

// parseSpeciesIDs reads repeated ?species= values. No values returns nil,
// which the ranking treats as "all species".
func parseSpeciesIDs(values []string) ([]int64, error) {
	if len(values) > maxTargetSpecies {
		return nil, fmt.Errorf("at most %d species are allowed", maxTargetSpecies)
	}
	var ids []int64
	for _, value := range values {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id <= 0 {
			return nil, errors.New("species must be a positive number")
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func handleGetRecommendations(logger *slog.Logger, recommendations recommendationService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()

		latitude, err := strconv.ParseFloat(query.Get("lat"), 64)
		if err != nil || latitude < -90 || latitude > 90 {
			http.Error(w, "lat must be a number between -90 and 90", http.StatusBadRequest)
			return
		}
		longitude, err := strconv.ParseFloat(query.Get("lon"), 64)
		if err != nil || longitude < -180 || longitude > 180 {
			http.Error(w, "lon must be a number between -180 and 180", http.StatusBadRequest)
			return
		}

		// A drive time limit replaces the radius when both are given.
		var radiusKM, maxMinutes float64
		if raw := query.Get("max_minutes"); raw != "" {
			maxMinutes, err = strconv.ParseFloat(raw, 64)
			// Written as a positive test so NaN, which fails every
			// comparison, is rejected too.
			if err != nil || !(maxMinutes > 0 && maxMinutes <= maxDriveMinutes) {
				http.Error(w, fmt.Sprintf("max_minutes must be a number above 0 and at most %d", maxDriveMinutes), http.StatusBadRequest)
				return
			}
		} else {
			radiusKM, err = strconv.ParseFloat(query.Get("radius_km"), 64)
			if err != nil || radiusKM <= 0 {
				http.Error(w, "radius_km must be a positive number, or max_minutes must be given", http.StatusBadRequest)
				return
			}
		}

		date := time.Now()
		if raw := query.Get("date"); raw != "" {
			date, err = time.Parse(dateParamLayout, raw)
			if err != nil {
				http.Error(w, "date must be in YYYY-MM-DD format", http.StatusBadRequest)
				return
			}
		}
		_, week := date.ISOWeek()

		speciesIDs, err := parseSpeciesIDs(query["species"])
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		results, err := recommendations.Recommend(r.Context(), birding.Params{
			Latitude:   latitude,
			Longitude:  longitude,
			RadiusKM:   radiusKM,
			MaxMinutes: maxMinutes,
			Week:       int16(week),
			Lang:       resolveLanguage(r),
			SpeciesIDs: speciesIDs,
		})
		switch {
		case errors.Is(err, birding.ErrTravelTimeUnavailable):
			http.Error(w, "travel time is not available; search by radius_km instead", http.StatusServiceUnavailable)
			return
		case errors.Is(err, birding.ErrNoRoadNearOrigin):
			http.Error(w, "no road near that location", http.StatusUnprocessableEntity)
			return
		case err != nil:
			logger.Error("ranking recommendations failed", slog.Any("error", err))
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		writeJSON(w, logger, http.StatusOK, results)
	})
}
