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
// opens before the first region closes.
func TestScanDuplicateStart(t *testing.T) {
	src := "<!-- apm:start -->\na\n<!-- apm:start -->\nb\n<!-- apm:end -->\n"
	f := newFile(t, src)
	_, diags := Scan(f, []config.ForeignRegion{apm})
	require.NotEmpty(t, diags)
	assert.Contains(t, diags[0].Message, "duplicate")
	assert.Equal(t, 3, diags[0].Line)
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
