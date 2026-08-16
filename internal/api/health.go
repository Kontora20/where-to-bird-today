package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// healthResponse is the body of GET /health.
//
// The `json:"status"` struct tag controls the wire name. Fields must be
// exported to be marshalled at all, and without a tag this one would serialise
// as "Status": there is no global naming policy to configure.
type healthResponse struct {
	Status string `json:"status"`
}

// handleHealth reports that the process is alive and serving.
//
// It deliberately does NOT check the database. This is a *liveness* probe:
// "is this process running?" Making it depend on Postgres would mean a brief
// DB blip causes the orchestrator to kill and restart otherwise-healthy
// containers. A separate /ready endpoint that does check dependencies belongs
// in Phase 1, once there is a database to check.
//
// Returning http.Handler from a function, rather than writing a bare handler
// function, is the idiomatic way to give a handler its dependencies — here,
// the logger, captured by the returned closure.
func handleHealth(logger *slog.Logger) http.Handler {
	// The request is named `_` because this handler reads nothing from it.
	// The signature is fixed by http.HandlerFunc, so the parameter cannot be
	// dropped; naming it `_` states the omission on purpose.
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, logger, http.StatusOK, healthResponse{Status: "ok"})
	})
}

// writeJSON serialises v and writes it as the response body.
//
// Note the order: marshal to a buffer FIRST, then set headers, then write.
// The tempting one-liner is json.NewEncoder(w).Encode(v), but that streams
// directly to the socket — by the time it discovers the value cannot be
// encoded, 200 OK and half a JSON document are already on the wire and it is
// too late to send a 500. Buffering costs one allocation and buys correct
// error handling.
//
// Once the status line is sent, an error can no longer be reported to the
// client, so a failed Write is logged rather than returned.
func writeJSON(w http.ResponseWriter, logger *slog.Logger, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		logger.Error("encoding response body failed", slog.Any("error", err))
		// http.Error writes text/plain, so the body is plain text rather than
		// JSON. Fine while /health is the only endpoint; Phase 1 should
		// introduce a proper JSON error shape shared by every handler.
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if _, err := w.Write(body); err != nil {
		logger.Error("writing response body failed", slog.Any("error", err))
	}
}
