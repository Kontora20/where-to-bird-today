package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"golang.org/x/text/language"

	"github.com/Kontora20/where-to-bird-today/internal/birding"
)

// dateParamLayout matches the plain "YYYY-MM-DD" a birder would type, not
// RFC 3339. This isn't a claim that time-of-day never matters for birding --
// it's that nothing downstream could use one yet: the EBD loader never reads
// TIME OBSERVATIONS STARTED (internal/ingest/ebd/ebd.go's requiredColumns),
// observations.observed_on is a date column with no time part, and
// species_seasonality aggregates by week_of_year only. Accepting a
// timestamp today would mean silently discarding the time half of it.
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

type recommendationService interface {
	Recommend(ctx context.Context, params birding.Params) ([]birding.Recommendation, error)
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
		radiusKM, err := strconv.ParseFloat(query.Get("radius_km"), 64)
		if err != nil || radiusKM <= 0 {
			http.Error(w, "radius_km must be a positive number", http.StatusBadRequest)
			return
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

		results, err := recommendations.Recommend(r.Context(), birding.Params{
			Latitude:  latitude,
			Longitude: longitude,
			RadiusKM:  radiusKM,
			Week:      int16(week),
			Lang:      resolveLanguage(r),
		})
		if err != nil {
			logger.Error("ranking recommendations failed", slog.Any("error", err))
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		writeJSON(w, logger, http.StatusOK, results)
	})
}
