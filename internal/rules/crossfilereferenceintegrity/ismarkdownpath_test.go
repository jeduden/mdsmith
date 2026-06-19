package crossfilereferenceintegrity

import "testing"

// TestIsMarkdownPath_UpperCaseZeroAllocs guards against strings.ToLower
// allocating when the extension is uppercase. strings.EqualFold compares
// without allocating; strings.ToLower + "==" allocates a new string for
// any extension that contains an upper-case byte.
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
