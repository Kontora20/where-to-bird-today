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

func TestSPACacheControl(t *testing.T) {
	t.Parallel()

	fsys := testSPAFS()
	fsys["_app/immutable/chunk.js"] = &fstest.MapFile{Data: []byte("chunk")}

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "hashed file is kept for good", path: "/_app/immutable/chunk.js", want: "public, max-age=31536000, immutable"},
		{name: "missing hashed file is not kept", path: "/_app/immutable/gone.js", want: "no-cache"},
		{name: "client route gets the shell, revalidated", path: "/hotspot/42", want: "no-cache"},
		{name: "other real file has no header", path: "/_app/immutable.js", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()

			handleSPA(fsys).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))

			if got := rec.Header().Get("Cache-Control"); got != tc.want {
				t.Errorf("Cache-Control = %q, want %q", got, tc.want)
			}
		})
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
