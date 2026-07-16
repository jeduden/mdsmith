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
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
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
		collectBodySyncPoints(content, headings, syncPoints, nil)
	})
	// After removing bytes.Split: 2 string() casts for 2 headings, no split alloc.
	assert.LessOrEqual(t, allocs, 2.0,
		"collectBodySyncPoints allocs: want ≤ 2 (string casts only), got %v", allocs)
}

// TestResolveBodySyncLine_NoPerLineStringAlloc confirms resolveBodySyncLine
// (the Fix-path counterpart to checkBodySync, run per body-sync-point line
// in fixBodySyncIn) does not allocate a string per scanned line. Before the
// fix, every line paid strings.TrimSpace(string(work[i])) even when it
// neither matched expected nor the field pattern; bytes.TrimSpace +
// bytes.Equal + re.Match stay in []byte for that common non-matching case,
// same convention as checkBodySync's expectedBytes precomputation.
func TestResolveBodySyncLine_NoPerLineStringAlloc(t *testing.T) {
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}

	bodyText := "Version: {version}"
	sp := syncPoint{
		Field:    "version",
		InBody:   true,
		BodyText: bodyText,
		compiled: buildFieldPattern(bodyText, nil),
	}
	docFM := map[string]any{"version": "2.0"}

	// 20 lines of unrelated prose: none equal the expected text or match
	// the field pattern, so the scan reaches every line without finding
	// anything to patch — the worst case for a per-line allocation.
	work := make([][]byte, 20)
	for i := range work {
		work[i] = []byte("unrelated prose that never matches the template")
	}

	allocs := testing.AllocsPerRun(100, func() {
		_, ok := resolveBodySyncLine(sp, docFM, work, 1, len(work))
		if ok {
			t.Fatal("expected no match against unrelated prose")
		}
	})
	t.Logf("resolveBodySyncLine allocs/op = %.0f", allocs)
	// 7 fixed allocs (expectedBytes, ParseCUEPath, etc.) plus zero per-line
	// allocations is the measured floor after the fix; the old
	// strings.TrimSpace(string(work[i])) cast paid one alloc per scanned
	// line on top of that (27 for this 20-line fixture).
	require.LessOrEqualf(t, allocs, 10.0,
		"resolveBodySyncLine allocs/op = %.0f; want no per-line string() allocation across 20 scanned lines",
		allocs)
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
