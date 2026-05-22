package linelength

import "testing"

// TestIsURLOnlyLine covers the byte-scan replacement for the old
// `urlOnlyRe.MatchString(strings.TrimSpace(string(line)))` shape
// the plan-195 alloc cut introduced. Each case mirrors a path the
// original regex would have decided, so a future regression that
// drifts from `^https?://\S+$`-over-TrimSpace fails here.
func TestIsURLOnlyLine(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"empty", "", false},
		{"only_spaces", "   ", false},
		{"http_only", "http://example.com", true},
		{"https_only", "https://example.com/path", true},
		{"trimmed_leading_space", "  https://example.com", true},
		{"trimmed_trailing_tab", "https://example.com\t", true},
		{"trimmed_both", "  http://x/y  ", true},
		{"non_url", "see https://example.com", false},
		{"http_with_internal_space", "https://example.com foo", false},
		{"empty_after_prefix", "https://", false},
		{"non_http_scheme", "ftp://example.com", false},
		{"bare_text", "hello world", false},
		{"trailing_newline_carriage", "https://x\r\n", true},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			got := isURLOnlyLine([]byte(c.in))
			if got != c.want {
				t.Fatalf("isURLOnlyLine(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}
