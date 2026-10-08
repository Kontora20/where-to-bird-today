package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
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

// recommender ranks hotspots for a search.
type recommender interface {
	Recommend(ctx context.Context, params birding.Params) ([]birding.Recommendation, error)
}

// weekFinder finds the best weeks of species in an area.
type weekFinder interface {
	AreaWeeks(ctx context.Context, params birding.AreaParams) (birding.AreaWeeks, error)
}

// speciesDetailer assembles the species detail panel.
type seasonFinder interface {
	InSeason(ctx context.Context, params birding.AreaParams, lang string) (birding.Season, error)
}

type speciesDetailer interface {
	SpeciesDetail(ctx context.Context, hotspotID, speciesID int64, lang string, date time.Time) (birding.SpeciesDetail, error)
}

// speciesSearcher finds species by name.
type speciesSearcher interface {
	SearchSpecies(ctx context.Context, query, lang string) ([]birding.SpeciesMatch, error)
}

// recommendationService is everything the router's handlers ask of the engine.
// Each handler takes only the part it calls, so its test needs a fake of only
// that.
type recommendationService interface {
	recommender
	weekFinder
	seasonFinder
	speciesDetailer
	speciesSearcher
}

// parseDate reads ?date=, which is today when absent.
func parseDate(query url.Values) (time.Time, error) {
	raw := query.Get("date")
	if raw == "" {
		return time.Now(), nil
	}
	date, err := time.Parse(dateParamLayout, raw)
	if err != nil {
		return time.Time{}, errors.New("date must be in YYYY-MM-DD format")
	}
	return date, nil
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

// search is what every request about an area asks: where, how far, which day
// and which species.
type search struct {
	latitude   float64
	longitude  float64
	radiusKM   float64
	maxMinutes float64
	date       time.Time
	speciesIDs []int64
}

// parseSearch reads the query of a request about an area. The error's text is
// meant for the client.
func parseSearch(query url.Values) (search, error) {
	var parsed search
	var err error

	// The ranges are written as positive tests so NaN is rejected too.
	parsed.latitude, err = strconv.ParseFloat(query.Get("lat"), 64)
	if err != nil || !(parsed.latitude >= -90 && parsed.latitude <= 90) {
		return search{}, errors.New("lat must be a number between -90 and 90")
	}
	parsed.longitude, err = strconv.ParseFloat(query.Get("lon"), 64)
	if err != nil || !(parsed.longitude >= -180 && parsed.longitude <= 180) {
		return search{}, errors.New("lon must be a number between -180 and 180")
	}

	// A drive time limit replaces the radius when both are given.
	if raw := query.Get("max_minutes"); raw != "" {
		parsed.maxMinutes, err = strconv.ParseFloat(raw, 64)
		// Written as a positive test so NaN, which fails every comparison, is
		// rejected too.
		if err != nil || !(parsed.maxMinutes > 0 && parsed.maxMinutes <= birding.MaxDriveMinutes) {
			return search{}, fmt.Errorf("max_minutes must be a number above 0 and at most %d", birding.MaxDriveMinutes)
		}
	} else {
		parsed.radiusKM, err = strconv.ParseFloat(query.Get("radius_km"), 64)
		if err != nil || !(parsed.radiusKM > 0 && parsed.radiusKM <= birding.MaxRadiusKM) {
			return search{}, fmt.Errorf("radius_km must be a number above 0 and at most %d, or max_minutes must be given", birding.MaxRadiusKM)
		}
	}

	parsed.date, err = parseDate(query)
	if err != nil {
		return search{}, err
	}
	parsed.speciesIDs, err = parseSpeciesIDs(query["species"])
	if err != nil {
		return search{}, err
	}
	return parsed, nil
}

// writeSearchError answers a failed search of an area. action names what was
// being done, for the log.
func writeSearchError(w http.ResponseWriter, logger *slog.Logger, err error, action string) {
	switch {
	case errors.Is(err, birding.ErrTravelTimeUnavailable):
		http.Error(w, "travel_time_unavailable", http.StatusServiceUnavailable)
	case errors.Is(err, birding.ErrNoRoadNearOrigin):
		http.Error(w, "no_road_near_origin", http.StatusUnprocessableEntity)
	case errors.Is(err, birding.ErrRoutingBusy):
		w.Header().Set("Retry-After", "2")
		http.Error(w, "routing_busy", http.StatusServiceUnavailable)
	default:
		logger.Error(action+" failed", slog.Any("error", err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func handleGetRecommendations(logger *slog.Logger, recommendations recommender) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parsed, err := parseSearch(r.URL.Query())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		results, err := recommendations.Recommend(r.Context(), birding.Params{
			Latitude:   parsed.latitude,
			Longitude:  parsed.longitude,
			RadiusKM:   parsed.radiusKM,
			MaxMinutes: parsed.maxMinutes,
			Week:       birding.WeekOfYear(parsed.date),
			Lang:       resolveLanguage(r),
			SpeciesIDs: parsed.speciesIDs,
		})
		if err != nil {
			writeSearchError(w, logger, err, "ranking recommendations")
			return
		}

		writeSearchJSON(w, logger, results)
	})
}

// handleGetAreaWeeks serves the best weeks of the requested species over the
// hotspots of an area: the same query as the recommendations, but at least one
// species is required.
func handleGetAreaWeeks(logger *slog.Logger, weeks weekFinder) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parsed, err := parseSearch(r.URL.Query())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(parsed.speciesIDs) == 0 {
			http.Error(w, "species is required", http.StatusBadRequest)
			return
		}

		found, err := weeks.AreaWeeks(r.Context(), birding.AreaParams{
			Latitude:   parsed.latitude,
			Longitude:  parsed.longitude,
			RadiusKM:   parsed.radiusKM,
			MaxMinutes: parsed.maxMinutes,
			SpeciesIDs: parsed.speciesIDs,
			Date:       parsed.date,
		})
		if err != nil {
			writeSearchError(w, logger, err, "finding best weeks")
			return
		}

		writeSearchJSON(w, logger, found)
	})
}

// handleGetInSeason serves the species most worth looking for in an area in the
// week of the date, and the area's checklists through the year. It takes the
// same query as the recommendations; species, if given, are ignored.
func handleGetInSeason(logger *slog.Logger, seasons seasonFinder) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parsed, err := parseSearch(r.URL.Query())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		found, err := seasons.InSeason(r.Context(), birding.AreaParams{
			Latitude:   parsed.latitude,
			Longitude:  parsed.longitude,
			RadiusKM:   parsed.radiusKM,
			MaxMinutes: parsed.maxMinutes,
			Date:       parsed.date,
		}, resolveLanguage(r))
		if err != nil {
			writeSearchError(w, logger, err, "finding species in season")
			return
		}

		writeSearchJSON(w, logger, found)
	})
}
