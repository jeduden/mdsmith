package requiredstructure

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// TestCheckBodySync_NoBytesPerLineAlloc confirms checkBodySync does not
// allocate a string per body line. A 6-line body section with no matching
// line must stay within budget: expectedBytes (1) + make(para) (1) +
// bytes.Join result (1) + fmt.Sprintf (1) + diagnostic slice (1) + 1
// margin = 6 allocs. The old two-loop code paid one string() per line
// in each loop = 12+ allocs, plus the []byte{' '} separator = 13+ total.
func TestCheckBodySync_NoBytesPerLineAlloc(t *testing.T) {
	src := "# Title\n\nline one\nline two\nline three\nline four\nline five\nline six\n"
	f, err := lint.NewFileFromSource("doc.md", []byte(src), true)
	require.NoError(t, err)

	dh := docHeading{Level: 1, Text: "Title", Line: 1}
	allHeadings := []docHeading{dh}

	allocs := testing.AllocsPerRun(100, func() {
		_ = checkBodySync(f, dh, 0, allHeadings, "no match here", "description")
	})
	assert.LessOrEqual(t, allocs, 6.0,
		"checkBodySync allocs: want ≤ 6, got %v", allocs)
}
