package mdpath

import (
	"reflect"
	"testing"
)

func TestExtensions(t *testing.T) {
	got := Extensions()
	want := []string{".md", ".markdown"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Extensions() = %v, want %v", got, want)
	}
	// Must return a fresh slice each call so a mutating caller cannot
	// corrupt the shared source of truth.
	got[0] = "mutated"
	if Extensions()[0] != ".md" {
		t.Fatalf("Extensions() returned a slice aliasing the package state")
	}
}

func TestFileGlobs(t *testing.T) {
	if got, want := FileGlobs(), []string{"*.md", "*.markdown"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("FileGlobs() = %v, want %v", got, want)
	}
}

func TestRecursiveGlobs(t *testing.T) {
	if got, want := RecursiveGlobs(), []string{"**/*.md", "**/*.markdown"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("RecursiveGlobs() = %v, want %v", got, want)
	}
}

func TestHasMarkdownExt(t *testing.T) {
	cases := []struct {
		ext  string
		want bool
	}{
		{".md", true},
		{".markdown", true},
		{".MD", true},
		{".Markdown", true},
		{".mdx", false},
		{".txt", false},
		{"", false},
		{"md", false}, // missing leading dot
	}
	for _, tc := range cases {
		if got := HasMarkdownExt(tc.ext); got != tc.want {
			t.Errorf("HasMarkdownExt(%q) = %v, want %v", tc.ext, got, tc.want)
		}
	}
}

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
