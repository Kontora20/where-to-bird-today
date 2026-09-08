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
// The fallback rewrites to "/", not "/index.html": FileServerFS has a
// documented special case that 301-redirects any request literally ending
// in "/index.html" to the bare directory path, to avoid two URLs serving
// the same page. Rewriting to "/index.html" walks straight into that
// redirect instead of getting the content; "/" serves it directly as the
// directory's normal index file.
func handleSPA(fsys fs.FS) http.Handler {
	fileServer := http.FileServerFS(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fs.Stat(fsys, strings.TrimPrefix(r.URL.Path, "/")); err != nil {
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}
