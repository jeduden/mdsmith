package requiredstructure

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCollectBodySyncPoints_NoByteSplitAlloc confirms collectBodySyncPoints
// no longer calls bytes.Split after the direct-scan rewrite. The only
// remaining allocations are the necessary string() casts for heading
// lines passed to headingMatchesLine — one per heading in the content.
// The content below has 2 headings and no {field} references, so we
// expect exactly 2 allocs (the two string conversions) rather than the
// original 3 (bytes.Split slice + 2 string conversions).
func TestCollectBodySyncPoints_NoByteSplitAlloc(t *testing.T) {
	content := []byte("## Section One\n\nSome prose without fields.\n\n## Section Two\n\nMore prose.\n")
	headings := []docHeading{
		{Text: "Section One", Level: 2, Line: 1},
		{Text: "Section Two", Level: 2, Line: 5},
	}
	syncPoints := make(map[int][]syncPoint)

	allocs := testing.AllocsPerRun(100, func() {
		for k := range syncPoints {
			delete(syncPoints, k)
		}
		collectBodySyncPoints(content, headings, syncPoints)
	})
	// After removing bytes.Split: 2 string() casts for 2 headings, no split alloc.
	assert.LessOrEqual(t, allocs, 2.0,
		"collectBodySyncPoints allocs: want ≤ 2 (string casts only), got %v", allocs)
}
