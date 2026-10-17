package api

import "net/http"

// handleWarmRouting is called by the page when someone asks for drive times,
// so the routing server is awake by the time the search arrives. It answers at
// once; warm does its work in the background and may be nil when there is no
// routing server.
func handleWarmRouting(warm func()) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if warm != nil {
			warm()
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
