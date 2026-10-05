package api

import "testing"

func TestIsBot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		userAgent string
		want      bool
	}{
		{userAgent: "", want: true},
		{userAgent: "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)", want: true},
		{userAgent: "Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko; compatible; GPTBot/1.2; +https://openai.com/gptbot)", want: true},
		{userAgent: "curl/8.9.1", want: true},
		{userAgent: "python-requests/2.32.3", want: true},
		{userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36", want: false},
		{userAgent: "Mozilla/5.0 (Linux; Android 13; CUBOT P80) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Mobile Safari/537.36", want: false},
	}

	for _, tc := range tests {
		if got := isBot(tc.userAgent); got != tc.want {
			t.Errorf("isBot(%q) = %t, want %t", tc.userAgent, got, tc.want)
		}
	}
}
