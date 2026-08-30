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

type hotspotStore interface {
	CreateHotspot(ctx context.Context, params store.CreateHotspotParams) (store.CreateHotspotRow, error)
	GetHotspot(ctx context.Context, id int64) (store.GetHotspotRow, error)
}

type createHotspotRequest struct {
	Name      string  `json:"name"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Region    string  `json:"region"`
	Source    string  `json:"source"`
	SourceRef string  `json:"source_ref"`
}

func handleCreateHotspot(logger *slog.Logger, hotspots hotspotStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req createHotspotRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.Name == "" || req.Region == "" || req.Source == "" || req.SourceRef == "" {
			http.Error(w, "name, region, source, and source_ref are required", http.StatusBadRequest)
			return
		}
		if req.Latitude < -90 || req.Latitude > 90 {
			http.Error(w, "latitude must be between -90 and 90", http.StatusBadRequest)
			return
		}
		if req.Longitude < -180 || req.Longitude > 180 {
			http.Error(w, "longitude must be between -180 and 180", http.StatusBadRequest)
			return
		}

		hotspot, err := hotspots.CreateHotspot(r.Context(), store.CreateHotspotParams{
			Name:      req.Name,
			Latitude:  req.Latitude,
			Longitude: req.Longitude,
			Region:    req.Region,
			Source:    req.Source,
			SourceRef: req.SourceRef,
		})
		if err != nil {
			logger.Error("creating hotspot failed", slog.Any("error", err))
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		writeJSON(w, logger, http.StatusCreated, hotspot)
	})
}

func handleGetHotspot(logger *slog.Logger, hotspots hotspotStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "id must be a number", http.StatusBadRequest)
			return
		}

		hotspot, err := hotspots.GetHotspot(r.Context(), id)
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "hotspot not found", http.StatusNotFound)
			return
		}
		if err != nil {
			logger.Error("reading hotspot failed", slog.Any("error", err))
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		writeJSON(w, logger, http.StatusOK, hotspot)
	})
}
