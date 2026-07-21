package foreignregion

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newFile(t *testing.T, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	return f
}

var apm = config.ForeignRegion{Start: "<!-- apm:start -->", End: "<!-- apm:end -->"}

// TestScanNoMarkers returns no ranges and no diagnostics when the file
// holds neither marker.
func TestScanNoMarkers(t *testing.T) {
	f := newFile(t, "# Title\n\nplain body\n")
	ranges, diags := Scan(f, []config.ForeignRegion{apm})
	assert.Empty(t, ranges)
	assert.Empty(t, diags)
}

// TestScanMatchedPair returns one inclusive range spanning the start
// marker line through the end marker line.
func TestScanMatchedPair(t *testing.T) {
	src := "# Title\n\n<!-- apm:start -->\nowned\ncontent\n<!-- apm:end -->\n\ntail\n"
	f := newFile(t, src)
	ranges, diags := Scan(f, []config.ForeignRegion{apm})
	require.Empty(t, diags)
	require.Len(t, ranges, 1)
	// Lines: 1 "# Title", 3 start, 6 end.
	assert.Equal(t, lint.LineRange{From: 3, To: 6}, ranges[0])
}

// TestScanUnmatchedStart reports a diagnostic on the start line when no
// end marker follows.
func TestScanUnmatchedStart(t *testing.T) {
	src := "# Title\n\n<!-- apm:start -->\nowned\n"
	f := newFile(t, src)
	ranges, diags := Scan(f, []config.ForeignRegion{apm})
	assert.Empty(t, ranges)
	require.Len(t, diags, 1)
	assert.Equal(t, 3, diags[0].Line)
	assert.Equal(t, RuleID, diags[0].RuleID)
	assert.Contains(t, diags[0].Message, "no matching end")
}

// TestScanOrphanedEnd reports a diagnostic when an end marker appears
// without a preceding start.
func TestScanOrphanedEnd(t *testing.T) {
	src := "# Title\n\n<!-- apm:end -->\n"
	f := newFile(t, src)
	ranges, diags := Scan(f, []config.ForeignRegion{apm})
	assert.Empty(t, ranges)
	require.Len(t, diags, 1)
	assert.Equal(t, 3, diags[0].Line)
	assert.Contains(t, diags[0].Message, "without a matching start")
}

// TestScanDuplicateStart reports a diagnostic when a second start marker
// opens before the first region closes, and still pairs the first start
// with the following end so that span stays protected (the duplicate
// marker only draws the diagnostic).
func TestScanDuplicateStart(t *testing.T) {
	src := "<!-- apm:start -->\na\n<!-- apm:start -->\nb\n<!-- apm:end -->\n"
	f := newFile(t, src)
	ranges, diags := Scan(f, []config.ForeignRegion{apm})
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "duplicate")
	assert.Equal(t, 3, diags[0].Line)
	// The first start (line 1) still pairs with the end (line 5): the
	// span is protected even though the region is flagged malformed.
	require.Len(t, ranges, 1)
	assert.Equal(t, lint.LineRange{From: 1, To: 5}, ranges[0])
}

// TestScanMultiplePairs returns a range for each independent matched
// pair of the same marker type.
func TestScanMultiplePairs(t *testing.T) {
	src := "<!-- apm:start -->\na\n<!-- apm:end -->\n\n<!-- apm:start -->\nb\n<!-- apm:end -->\n"
	f := newFile(t, src)
	ranges, diags := Scan(f, []config.ForeignRegion{apm})
	require.Empty(t, diags)
	require.Len(t, ranges, 2)
	assert.Equal(t, lint.LineRange{From: 1, To: 3}, ranges[0])
	assert.Equal(t, lint.LineRange{From: 5, To: 7}, ranges[1])
}

// TestScanIndentedMarkerMatches matches a marker even when the line is
// indented, since markers are compared against the trimmed line.
func TestScanIndentedMarkerMatches(t *testing.T) {
	src := "  <!-- apm:start -->\nx\n  <!-- apm:end -->\n"
	f := newFile(t, src)
	ranges, diags := Scan(f, []config.ForeignRegion{apm})
	require.Empty(t, diags)
	require.Len(t, ranges, 1)
	assert.Equal(t, lint.LineRange{From: 1, To: 3}, ranges[0])
}

// TestScanMultipleRegions_AllocsScaleWithLinesNotRegions pins
// docs/development/high-performance-go.md's "stay in []byte" and
// "skip redundant re-scanning" patterns for Scan with more than one
// declared region. Before this test, Scan called scanOne once per
// region, and each scanOne call re-walked the whole f.Lines slice
// converting every line via strings.TrimSpace(string(line)) — an
// O(regions x lines) cost with one string allocation per line per
// region, even though a single per-line trim can be checked against
// every region's markers in the same pass.
//
// The assertion: allocs for 5 regions must not exceed roughly what 1
// region costs by more than a small per-region constant (each region
// still needs its own open-marker state and, on this fixture, no
// diagnostics) — a multi-pass implementation would instead scale
// allocs linearly with len(f.Lines) times len(regions).
func TestScanMultipleRegions_AllocsScaleWithLinesNotRegions(t *testing.T) {
	var src []byte
	for i := 0; i < 100; i++ {
		src = append(src, []byte("A representative line of prose in the file.\n")...)
	}
	f := newFile(t, string(src))

	oneRegion := []config.ForeignRegion{apm}
	fiveRegions := []config.ForeignRegion{
		apm,
		{Start: "<!-- r2:start -->", End: "<!-- r2:end -->"},
		{Start: "<!-- r3:start -->", End: "<!-- r3:end -->"},
		{Start: "<!-- r4:start -->", End: "<!-- r4:end -->"},
		{Start: "<!-- r5:start -->", End: "<!-- r5:end -->"},
	}

	oneAllocs := testing.AllocsPerRun(20, func() { Scan(f, oneRegion) })
	fiveAllocs := testing.AllocsPerRun(20, func() { Scan(f, fiveRegions) })

	// A single-pass, []byte-only scan pays a near-constant per-region
	// setup cost (a states slice entry) regardless of file size; a
	// multi-pass, string()-converting scan pays roughly 5x oneAllocs
	// (100 lines re-converted per extra region). 3x oneAllocs+10 is
	// comfortably below the multi-pass cost and above single-pass
	// noise.
	assert.LessOrEqualf(t, fiveAllocs, oneAllocs*3+10,
		"Scan with 5 regions allocs (%v) must not scale with lines-per-region "+
			"(1-region allocs: %v) — each region should reuse one per-line trim",
		fiveAllocs, oneAllocs)
}
