package catalog

import "testing"

// TestExtractPlaceholderFields_PresizedAllocs pins
// docs/development/high-performance-go.md's "pre-size slices" pattern:
// extractPlaceholderFields knows fieldinterp.Fields's result count up
// front (via `all`, already used to presize `seen`), so `fields` should
// be allocated once via make([]string, 0, len(all)) instead of growing
// from a nil slice across repeated append calls.
func TestExtractPlaceholderFields_PresizedAllocs(t *testing.T) {
	row := "- [{summary}]({filename}) by {author}, tagged {tag}"
	allocs := testing.AllocsPerRun(200, func() {
		_ = extractPlaceholderFields(row)
	})
	if allocs > 11 {
		t.Fatalf("extractPlaceholderFields allocs/op = %v, want <= 11", allocs)
	}
}
