package api

import (
	"io/fs"
	"net/http"
	"strings"
)

// handleSPA serves the built frontend from fsys, falling back to
// index.html for any path that isn't a real file. http.FileServerFS alone
// serves real files and 404s on the rest -- client-side routes need the
// fallback so a browser refresh on e.g. /some/client/route still works,
// rather than hitting a Go 404 for a path that only the SPA's own router
// understands.
//
// Files under _app/immutable carry a hash of their content in the name, so a
// browser may keep them for good. The header is set only when the file exists:
// a missing one falls back to index.html, which must never be kept that long.
// The index itself is revalidated every time, so a new deploy shows at once.
//
// The fallback rewrites to "/", not "/index.html": FileServerFS has a
// documented special case that 301-redirects any request literally ending
// in "/index.html" to the bare directory path, to avoid two URLs serving
// the same page. Rewriting to "/index.html" walks straight into that
// redirect instead of getting the content; "/" serves it directly as the
// directory's normal index file.
func handleSPA(fsys fs.FS) http.Handler {
	fileServer := http.FileServerFS(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if _, err := fs.Stat(fsys, name); err != nil {
			r = r.Clone(r.Context())
			r.URL.Path = "/"
			w.Header().Set("Cache-Control", "no-cache")
		} else if strings.HasPrefix(name, "_app/immutable/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		fileServer.ServeHTTP(w, r)
	})
}
