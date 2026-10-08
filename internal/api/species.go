package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// maxSpeciesQueryRunes is the longest search text accepted. The search
// lowercases and unaccents every species name for each request, so what it
// costs grows with the length of the text.
const maxSpeciesQueryRunes = 64

// validSpeciesQuery reports why query cannot be searched for, or "" when it
// can. Postgres rejects a NUL byte in text, which would answer 500.
func validSpeciesQuery(query string) string {
	switch {
	case !utf8.ValidString(query) || strings.ContainsRune(query, 0):
		return "q is not valid text"
	case utf8.RuneCountInString(query) > maxSpeciesQueryRunes:
		return "q is too long"
	}
	return ""
}

// handleSearchSpecies serves the target-species picker: species whose name
// contains ?q=, named in the request's language.
func handleSearchSpecies(logger *slog.Logger, species speciesSearcher) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := strings.TrimSpace(r.URL.Query().Get("q"))
		if query == "" {
			http.Error(w, "q is required", http.StatusBadRequest)
			return
		}
		if reason := validSpeciesQuery(query); reason != "" {
			http.Error(w, reason, http.StatusBadRequest)
			return
		}

		matches, err := species.SearchSpecies(r.Context(), query, resolveLanguage(r))
		if err != nil {
			logger.Error("searching species failed", slog.Any("error", err))
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		writeSearchJSON(w, logger, matches)
	})
}

// handleGetSpeciesDetail serves the species detail panel's contents (ADR
// 0004): name, outbound reference links, one photo, and the requested
// hotspot's week-by-week seasonality and hour-by-hour time of day for the
// species. Both are measured per hotspot, so the hotspot is part of the URL,
// not optional. ?date= sets the day the time of day is shown at.
func handleGetSpeciesDetail(logger *slog.Logger, species speciesDetailer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hotspotID, err := strconv.ParseInt(r.PathValue("hotspotId"), 10, 64)
		if err != nil {
			http.Error(w, "hotspotId must be a number", http.StatusBadRequest)
			return
		}
		speciesID, err := strconv.ParseInt(r.PathValue("speciesId"), 10, 64)
		if err != nil {
			http.Error(w, "speciesId must be a number", http.StatusBadRequest)
			return
		}

		date, err := parseDate(r.URL.Query())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		detail, err := species.SpeciesDetail(r.Context(), hotspotID, speciesID, resolveLanguage(r), date)
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "species not found", http.StatusNotFound)
			return
		}
		if err != nil {
			logger.Error("loading species detail failed", slog.Any("error", err))
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		writeSearchJSON(w, logger, detail)
	})
}
