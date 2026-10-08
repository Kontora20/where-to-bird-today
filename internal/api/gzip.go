package api

import (
	"compress/gzip"
	"net/http"
	"strings"
)

// gzipResponses compresses the response for a client that accepts it. A search
// answer is mostly repeated field names, and a 300 km search is about 180 KB
// of JSON that compresses to a fraction of that.
func gzipResponses(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Accept-Encoding")
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		compressed := &gzipWriter{ResponseWriter: w, body: gzip.NewWriter(w)}
		next.ServeHTTP(compressed, r)
		// Closing writes the end of the stream, which is only wanted when a
		// response was started and marked as compressed.
		if compressed.wroteHeader {
			_ = compressed.body.Close()
		}
	})
}

// gzipWriter sends what is written to it through body. The embedded
// ResponseWriter still handles the status and the headers.
type gzipWriter struct {
	http.ResponseWriter
	body        *gzip.Writer
	wroteHeader bool
}

// WriteHeader marks the response as compressed just before the status is
// sent, and not earlier: http.Error edits the headers first and would leave
// a body that is compressed without the header that says so.
func (w *gzipWriter) WriteHeader(status int) {
	if !w.wroteHeader {
		w.wroteHeader = true
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Del("Content-Length")
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *gzipWriter) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.body.Write(data)
}
