package httpsurl

import "testing"

func TestNormalize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "https is kept", raw: "https://commons.wikimedia.org/wiki/User:Alun", want: "https://commons.wikimedia.org/wiki/User:Alun"},
		{name: "http is upgraded", raw: "http://creativecommons.org/licenses/by-sa/3.0/", want: "https://creativecommons.org/licenses/by-sa/3.0/"},
		{name: "scheme-less is upgraded", raw: "//commons.wikimedia.org/wiki/User:PierreSelim", want: "https://commons.wikimedia.org/wiki/User:PierreSelim"},
		{name: "surrounding space is trimmed", raw: "  https://example.org/a  ", want: "https://example.org/a"},
		{name: "javascript", raw: "javascript:alert(1)", want: ""},
		{name: "javascript in capitals", raw: "JavaScript:alert(1)", want: ""},
		{name: "javascript behind a tab", raw: "java\tscript:alert(1)", want: ""},
		{name: "data", raw: "data:text/html,<script>alert(1)</script>", want: ""},
		{name: "ftp", raw: "ftp://example.org/file", want: ""},
		{name: "no host", raw: "https:///path", want: ""},
		{name: "relative path", raw: "/wiki/User:Alun", want: ""},
		{name: "empty", raw: "", want: ""},
		{name: "not a URL", raw: "https://%zz", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := Normalize(tc.raw); got != tc.want {
				t.Errorf("Normalize(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}
