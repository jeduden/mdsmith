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

// TestRenderMinimal_ScaleInvariantAllocs pins
// docs/development/high-performance-go.md's "strings.Builder over +.
// Call Grow(n) first if you know the final size": renderMinimal used
// to build each row as a `+`-concatenated string before a single
// WriteString call, which (a) let an un-Grow'n Builder rediscover its
// final size through repeated doubling and (b) still cost one
// allocation per row for the concatenated string itself, even once (a)
// was fixed by pre-sizing. A catalog directive can match dozens of
// files (docs/features/progressive-disclosure.md), so allocation count
// must not scale with entry count. Both fixes together bring it to one
// allocation total, independent of how many entries are rendered.
func TestRenderMinimal_ScaleInvariantAllocs(t *testing.T) {
	few := make([]fileEntry, 2)
	many := make([]fileEntry, 50)
	for _, entries := range [][]fileEntry{few, many} {
		for i := range entries {
			entries[i] = fileEntry{fields: map[string]any{"filename": "docs/some/deep/path/file.md"}}
		}
	}

	fewAllocs := testing.AllocsPerRun(200, func() { _ = renderMinimal(few) })
	manyAllocs := testing.AllocsPerRun(200, func() { _ = renderMinimal(many) })

	if manyAllocs > fewAllocs {
		t.Fatalf("renderMinimal allocs/op scale with entry count: %v (n=2) vs %v (n=50)",
			fewAllocs, manyAllocs)
	}
	const wantMax = 1
	if manyAllocs > wantMax {
		t.Fatalf("renderMinimal allocs/op = %v, want <= %v", manyAllocs, wantMax)
	}
}
