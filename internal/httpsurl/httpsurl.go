// Package httpsurl cleans up links that come from outside before they are
// stored or sent to a browser.
package httpsurl

import (
	"net/url"
	"strings"
)

// Normalize returns raw as an https link, or "" when it is not one. The two
// forms Wikimedia Commons uses for real links are upgraded: "http://..." and
// the scheme-less "//...". Anything else, such as "javascript:" or "data:",
// would run or load something when followed, and is dropped.
func Normalize(raw string) string {
	trimmed := strings.TrimSpace(raw)
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return ""
	}
	schemeLess := parsed.Scheme == "" && strings.HasPrefix(trimmed, "//")
	if schemeLess || parsed.Scheme == "http" {
		parsed.Scheme = "https"
	}
	if parsed.Scheme != "https" || parsed.Host == "" {
		return ""
	}
	return parsed.String()
}
