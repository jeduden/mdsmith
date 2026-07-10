package duplicatedcontent

import "testing"

// TestIsMarkdownPath_UpperCaseZeroAllocs guards against strings.ToLower
// allocating in isMarkdownPath, which runs once per sibling file while
// indexing the corpus for MDS-duplicated-content — O(files) per run
// (docs/development/high-performance-go.md#strings-and-bytes). ToLower
// allocates a new copy of the whole path whenever it contains an
// upper-case byte; strings.EqualFold compares the extension without
// allocating, matching the fix already applied in
// crossfilereferenceintegrity.isMarkdownPath.
func TestIsMarkdownPath_UpperCaseZeroAllocs(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	allocs := testing.AllocsPerRun(100, func() {
		_ = isMarkdownPath("docs/README.MD")
	})
	if allocs > 0 {
		t.Fatalf("isMarkdownPath: expected 0 allocs/op for upper-case extension, got %.0f", allocs)
	}
}
