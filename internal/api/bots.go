package api

import (
	"net/http"
	"strings"
)

// botUserAgentParts are lowercase parts of the user agents of crawlers and
// scripted HTTP clients. "bot/" matches Googlebot/, GPTBot/, bingbot/ and
// most others without matching phone names such as CUBOT.
var botUserAgentParts = []string{
	"bot/", "crawler", "spider", "headlesschrome",
	"curl/", "wget/", "python-", "go-http-client", "scrapy", "java/", "okhttp",
}

// blockBots answers 403 to requests with no user agent or a crawler's or
// scripted client's, so they cannot run searches. It only stops clients that
// say what they are; the rate limit handles the rest.
func blockBots(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isBot(r.UserAgent()) {
			http.Error(w, "automated clients are not allowed", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isBot(userAgent string) bool {
	if strings.TrimSpace(userAgent) == "" {
		return true
	}
	userAgent = strings.ToLower(userAgent)
	for _, part := range botUserAgentParts {
		if strings.Contains(userAgent, part) {
			return true
		}
	}
	return false
}
