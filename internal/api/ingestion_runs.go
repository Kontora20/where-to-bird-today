package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/Kontora20/where-to-bird-today/internal/store"
)

// ingestionRunStore is the two queries the handlers below need. Declared here,
// beside its only caller, rather than in store: the store package should not
// have to know which of its many generated queries the API layer happens to
// use.
type ingestionRunStore interface {
	CreateIngestionRun(ctx context.Context, source string) (store.IngestionRun, error)
	GetIngestionRun(ctx context.Context, id int64) (store.IngestionRun, error)
}

type createIngestionRunRequest struct {
	Source string `json:"source"`
}

func handleCreateIngestionRun(logger *slog.Logger, runs ingestionRunStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req createIngestionRunRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.Source == "" {
			http.Error(w, "source is required", http.StatusBadRequest)
			return
		}

		run, err := runs.CreateIngestionRun(r.Context(), req.Source)
		if err != nil {
			logger.Error("creating ingestion run failed", slog.Any("error", err))
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		writeJSON(w, logger, http.StatusCreated, run)
	})
}

func handleGetIngestionRun(logger *slog.Logger, runs ingestionRunStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "id must be a number", http.StatusBadRequest)
			return
		}

		run, err := runs.GetIngestionRun(r.Context(), id)
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "ingestion run not found", http.StatusNotFound)
			return
		}
		if err != nil {
			logger.Error("reading ingestion run failed", slog.Any("error", err))
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		writeJSON(w, logger, http.StatusOK, run)
	})
}
