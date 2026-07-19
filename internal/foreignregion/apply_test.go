package foreignregion

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func apmCfg() *config.Config {
	return &config.Config{ForeignRegions: []config.ForeignRegion{apm}}
}

// TestScanNilFile returns nothing for a nil *File rather than panicking.
func TestScanNilFile(t *testing.T) {
	ranges, diags := Scan(nil, []config.ForeignRegion{apm})
	assert.Nil(t, ranges)
	assert.Nil(t, diags)
}

// TestApplyExtendsRangesAndReturnsDiags appends the matched-pair spans to
// f.GeneratedRanges and returns the malformed-region diagnostics.
func TestApplyExtendsRangesAndReturnsDiags(t *testing.T) {
	// A matched pair plus a trailing unmatched start (malformed).
	src := "<!-- apm:start -->\nbody\n<!-- apm:end -->\n<!-- apm:start -->\ndangling\n"
	f := newFile(t, src)
	diags := Apply(f, apmCfg(), "test.md")
	require.Len(t, f.GeneratedRanges, 1)
	assert.Equal(t, lint.LineRange{From: 1, To: 3}, f.GeneratedRanges[0])
	require.Len(t, diags, 1)
	assert.Equal(t, "test.md", diags[0].File)
	assert.Contains(t, diags[0].Message, "no matching end")
}

// TestApplyNoRegions leaves GeneratedRanges untouched and returns no
// diagnostics when the config declares no marker pairs.
func TestApplyNoRegions(t *testing.T) {
	f := newFile(t, "# Title\n\nbody\n")
	diags := Apply(f, &config.Config{}, "test.md")
	assert.Empty(t, f.GeneratedRanges)
	assert.Nil(t, diags)
}

// TestAppendRangesPopulatesWithoutDiags extends the exclusion set but
// discards the malformed diagnostics (the read-only RunSource path).
func TestAppendRangesPopulatesWithoutDiags(t *testing.T) {
	src := "<!-- apm:start -->\nbody\n<!-- apm:end -->\n"
	f := newFile(t, src)
	AppendRanges(f, apmCfg(), "test.md")
	require.Len(t, f.GeneratedRanges, 1)
	assert.Equal(t, lint.LineRange{From: 1, To: 3}, f.GeneratedRanges[0])
}

// TestDiagnosticsReturnsWithoutMutating rebuilds the malformed diagnostics
// without touching f.GeneratedRanges.
func TestDiagnosticsReturnsWithoutMutating(t *testing.T) {
	src := "<!-- apm:end -->\n"
	f := newFile(t, src)
	diags := Diagnostics(f, apmCfg(), "test.md")
	assert.Empty(t, f.GeneratedRanges)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "without a matching start")
	assert.Equal(t, "test.md", diags[0].File)
}

// TestDiagnosticsNoRegions returns nil when no marker pairs apply.
func TestDiagnosticsNoRegions(t *testing.T) {
	f := newFile(t, "# Title\n")
	assert.Nil(t, Diagnostics(f, &config.Config{}, "test.md"))
}
