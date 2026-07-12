package mdpath

import "testing"

func TestIsMarkdownPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"docs/README.md", true},
		{"docs/README.MD", true},
		{"docs/README.Markdown", true},
		{"docs/README.mdx", false},
		{"docs/image.png", false},
		{"docs/README", false},
		{"README.md", true},
		{"", false},
		{".md", true},
		// Windows-native paths (filepath.FromSlash output): a dot must
		// be found relative to the LAST separator of either kind, not
		// just '/'. Regression cases for the path.Ext-only bug described
		// in IsMarkdownPath's doc comment.
		{`docs\README.md`, true},
		{`docs\README.MD`, true},
		{`some.dir\README`, false},
		{`some.dir\README.md`, true},
		{`a/b.dir\c\README.md`, true},
	}
	for _, tc := range cases {
		if got := IsMarkdownPath(tc.path); got != tc.want {
			t.Errorf("IsMarkdownPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

// TestIsMarkdownPath_UpperCaseZeroAllocs guards against an allocating
// case-fold creeping back in: strings.ToLower allocates a new string
// whenever the input contains an upper-case byte, while strings.EqualFold
// compares without allocating.
func TestIsMarkdownPath_UpperCaseZeroAllocs(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	allocs := testing.AllocsPerRun(100, func() {
		_ = IsMarkdownPath("docs/README.MD")
	})
	if allocs > 0 {
		t.Fatalf("IsMarkdownPath: expected 0 allocs/op for upper-case extension, got %.0f", allocs)
	}
}
