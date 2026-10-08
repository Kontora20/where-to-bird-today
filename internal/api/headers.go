package api

import "net/http"

// securityHeaders sets the response headers that tell a browser what a page
// may do. The content security policy is not here: it names a hash for each
// inline script in the built page, so SvelteKit writes it into index.html
// (see svelte.config.js).
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("X-Frame-Options", "DENY")
		header.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		header.Set("Permissions-Policy", "geolocation=(self), camera=(), microphone=()")
		next.ServeHTTP(w, r)
	})
}
