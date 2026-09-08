package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func testSPAFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":        &fstest.MapFile{Data: []byte("<html>app shell</html>")},
		"_app/immutable.js": &fstest.MapFile{Data: []byte("console.log('chunk')")},
	}
}

func TestSPAServesRealFiles(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/_app/immutable.js", nil)
	rec := httptest.NewRecorder()

	handleSPA(testSPAFS()).ServeHTTP(rec, req)

	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if got, want := rec.Body.String(), "console.log('chunk')"; got != want {
		t.Errorf("body = %q, want %q (a real file should be served as-is, not rewritten)", got, want)
	}
}

// TestSPAFallsBackToIndexForUnknownPaths is the actual point of handleSPA:
// a client-side route like /hotspot/42 isn't a real file in the build, but a
// browser navigating there directly (or refreshing) still needs the app
// shell back, not a 404, so the SPA's own router can take over client-side.
func TestSPAFallsBackToIndexForUnknownPaths(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/hotspot/42", nil)
	rec := httptest.NewRecorder()

	handleSPA(testSPAFS()).ServeHTTP(rec, req)

	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if got, want := rec.Body.String(), "<html>app shell</html>"; got != want {
		t.Errorf("body = %q, want %q (unknown path should fall back to index.html)", got, want)
	}
}
