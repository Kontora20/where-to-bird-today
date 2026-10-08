package api

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGzipResponses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		acceptsGzip  bool
		handler      http.HandlerFunc
		wantStatus   int
		wantEncoding string
		wantBody     string
	}{
		{
			name:         "compressed when accepted",
			acceptsGzip:  true,
			handler:      func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("[1,2,3]")) },
			wantStatus:   http.StatusOK,
			wantEncoding: "gzip",
			wantBody:     "[1,2,3]",
		},
		{
			name:        "plain when not accepted",
			acceptsGzip: false,
			handler:     func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("[1,2,3]")) },
			wantStatus:  http.StatusOK,
			wantBody:    "[1,2,3]",
		},
		{
			name:         "an error is compressed and marked as such",
			acceptsGzip:  true,
			handler:      func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "q is too long", http.StatusBadRequest) },
			wantStatus:   http.StatusBadRequest,
			wantEncoding: "gzip",
			wantBody:     "q is too long\n",
		},
		{
			name:        "an empty answer is left alone",
			acceptsGzip: true,
			handler:     func(http.ResponseWriter, *http.Request) {},
			wantStatus:  http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/species?q=x", nil)
			if tc.acceptsGzip {
				req.Header.Set("Accept-Encoding", "gzip, deflate")
			}
			rec := httptest.NewRecorder()

			gzipResponses(tc.handler).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if got := rec.Header().Get("Content-Encoding"); got != tc.wantEncoding {
				t.Errorf("Content-Encoding = %q, want %q", got, tc.wantEncoding)
			}
			if got := rec.Header().Get("Vary"); got != "Accept-Encoding" {
				t.Errorf("Vary = %q, want %q", got, "Accept-Encoding")
			}

			body := rec.Body.String()
			if tc.wantEncoding == "gzip" {
				reader, err := gzip.NewReader(strings.NewReader(body))
				if err != nil {
					t.Fatalf("body is not gzip: %v", err)
				}
				decoded, err := io.ReadAll(reader)
				if err != nil {
					t.Fatalf("reading gzip body: %v", err)
				}
				body = string(decoded)
			}
			if body != tc.wantBody {
				t.Errorf("body = %q, want %q", body, tc.wantBody)
			}
		})
	}
}
