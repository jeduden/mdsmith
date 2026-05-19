package tablestructure

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rules/tableformat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func check(t *testing.T, style, src string) []lint.Diagnostic {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	r := &Rule{Style: style}
	return r.Check(f)
}

func fix(t *testing.T, style, src string) string {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	r := &Rule{Style: style}
	return string(r.Fix(f))
}

func TestMD058CRLFBlankLine(t *testing.T) {
	// A CRLF file must get a CRLF blank line inserted, not a bare
	// LF, so `mdsmith fix` does not introduce mixed line endings.
	src := "# T\r\nText.\r\n| A | B |\r\n| - | - |\r\nMore.\r\n"
	got := fix(t, StyleConsistent, src)
	want := "# T\r\nText.\r\n\r\n| A | B |\r\n| - | - |\r\n\r\nMore.\r\n"
	assert.Equal(t, want, got)
	assert.NotContains(t, got, "\r\n\n", "no bare-LF blank line")
}

func TestIdentity(t *testing.T) {
	r := &Rule{Style: StyleConsistent}
	assert.Equal(t, "MDS060", r.ID())
	assert.Equal(t, "table-structure", r.Name())
	assert.Equal(t, "table", r.Category())
}

func TestConsistentBorderedClean(t *testing.T) {
	src := "# T\n\n| A | B |\n| - | - |\n| 1 | 2 |\n"
	assert.Empty(t, check(t, StyleConsistent, src))
}

func TestConsistentBorderlessClean(t *testing.T) {
	src := "# T\n\nA | B\n- | -\n1 | 2\n"
	assert.Empty(t, check(t, StyleConsistent, src))
}

func TestMD055MixedFlaggedConsistent(t *testing.T) {
	// Borderless header -> consistent expects no edge pipes; the
	// bordered data row (line 5) is the only violation.
	src := "# T\n\nA | B\n- | -\n| 1 | 2 |\n3 | 4\n"
	diags := check(t, StyleConsistent, src)
	require.Len(t, diags, 1)
	assert.Equal(t, 5, diags[0].Line)
	assert.Equal(t, 1, diags[0].Column)
	assert.Equal(t,
		"table pipe style; expected no leading or trailing pipes",
		diags[0].Message)
}

func TestMD055FixNormalizesToConsistent(t *testing.T) {
	src := "# T\n\nA | B\n- | -\n| 1 | 2 |\n3 | 4\n"
	got := fix(t, StyleConsistent, src)
	want := "# T\n\nA | B\n- | -\n1 | 2\n3 | 4\n"
	assert.Equal(t, want, got)
	assert.Empty(t, check(t, StyleConsistent, got), "fixed output must be clean")
}

func TestMD055LeadingAndTrailingStyle(t *testing.T) {
	src := "# T\n\nA | B\n- | -\n1 | 2\n"
	diags := check(t, StyleLeadingAndTrailing, src)
	require.Len(t, diags, 3) // header, separator, one data row
	for _, d := range diags {
		assert.Equal(t,
			"table pipe style; expected leading and trailing pipes",
			d.Message)
	}
	got := fix(t, StyleLeadingAndTrailing, src)
	assert.Equal(t, "# T\n\n| A | B |\n| - | - |\n| 1 | 2 |\n", got)
}

func TestMD055NoLeadingOrTrailingStyle(t *testing.T) {
	src := "# T\n\n| A | B |\n| - | - |\n| 1 | 2 |\n"
	got := fix(t, StyleNoLeadingOrTrailing, src)
	assert.Equal(t, "# T\n\nA | B\n- | -\n1 | 2\n", got)
	assert.Empty(t, check(t, StyleNoLeadingOrTrailing, got))
}

func TestMD056FlaggedNotFixed(t *testing.T) {
	// Row 5 has one cell where the header has two.
	src := "# T\n\n| A | B |\n| - | - |\n| 1 |\n| 3 | 4 |\n"
	diags := check(t, StyleConsistent, src)
	require.Len(t, diags, 1)
	assert.Equal(t, 5, diags[0].Line)
	assert.Equal(t, "table column count; expected 2, got 1", diags[0].Message)

	// Fix must not invent the missing cell: the short row survives.
	got := fix(t, StyleConsistent, src)
	assert.Equal(t, src, got)
}

func TestMD056TooManyCells(t *testing.T) {
	src := "# T\n\n| A | B |\n| - | - |\n| 1 | 2 | 3 |\n"
	diags := check(t, StyleConsistent, src)
	require.Len(t, diags, 1)
	assert.Equal(t, "table column count; expected 2, got 3", diags[0].Message)
}

func TestMD058MissingBefore(t *testing.T) {
	src := "# T\n\nText paragraph.\n| A | B |\n| - | - |\n\n"
	diags := check(t, StyleConsistent, src)
	require.Len(t, diags, 1)
	assert.Equal(t, 4, diags[0].Line)
	assert.Equal(t, "missing blank line before table", diags[0].Message)

	got := fix(t, StyleConsistent, src)
	assert.Equal(t, "# T\n\nText paragraph.\n\n| A | B |\n| - | - |\n\n", got)
}

func TestMD058MissingAfter(t *testing.T) {
	src := "# T\n\n| A | B |\n| - | - |\nText paragraph.\n"
	diags := check(t, StyleConsistent, src)
	require.Len(t, diags, 1)
	assert.Equal(t, 4, diags[0].Line)
	assert.Equal(t, "missing blank line after table", diags[0].Message)

	got := fix(t, StyleConsistent, src)
	assert.Equal(t, "# T\n\n| A | B |\n| - | - |\n\nText paragraph.\n", got)
}

func TestMD058StartAndEndOfDocOK(t *testing.T) {
	src := "| A | B |\n| - | - |\n| 1 | 2 |\n"
	assert.Empty(t, check(t, StyleConsistent, src))
}

func TestMD058BothSidesFixed(t *testing.T) {
	src := "Intro.\n| A | B |\n| - | - |\nOutro.\n"
	got := fix(t, StyleConsistent, src)
	assert.Equal(t, "Intro.\n\n| A | B |\n| - | - |\n\nOutro.\n", got)
	assert.Empty(t, check(t, StyleConsistent, got))
}

func TestSkipsCodeBlock(t *testing.T) {
	src := "# T\n\n```\n| A | B |\n|---|\nText\n```\n"
	assert.Empty(t, check(t, StyleConsistent, src))
}

func TestSetextHeadingNotTable(t *testing.T) {
	// `Title` over `---` is a setext H2, not a table (no pipes).
	src := "Title\n---\n\nBody.\n"
	assert.Empty(t, check(t, StyleConsistent, src))
}

func TestApplySettings(t *testing.T) {
	r := &Rule{Style: StyleConsistent}
	require.NoError(t, r.ApplySettings(map[string]any{"style": StyleLeadingAndTrailing}))
	assert.Equal(t, StyleLeadingAndTrailing, r.Style)

	require.Error(t, r.ApplySettings(map[string]any{"style": "bogus"}))
	require.Error(t, r.ApplySettings(map[string]any{"style": 7}))
	require.Error(t, r.ApplySettings(map[string]any{"unknown": "x"}))
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{Style: StyleConsistent}
	assert.Equal(t, map[string]any{"style": StyleConsistent}, r.DefaultSettings())
}

// TestLoopStabilityWithMDS025 reproduces the fix engine's per-pass
// order (rules sorted by ID, so MDS025 runs before MDS060) and asserts
// the combined fix converges within the engine's 10-pass budget and
// holds steady afterward.
func TestLoopStabilityWithMDS025(t *testing.T) {
	src := "# T\nText.\n| A | B |\n| - | - |\n| 1 | 2 |\nMore text.\n"
	tf := &tableformat.Rule{Pad: 1}
	ts := &Rule{Style: StyleConsistent}

	current := src
	const maxPasses = 10
	passes := 0
	for ; passes < maxPasses; passes++ {
		before := current
		for _, fr := range []interface {
			Check(*lint.File) []lint.Diagnostic
			Fix(*lint.File) []byte
		}{tf, ts} {
			f, err := lint.NewFile("t.md", []byte(current))
			require.NoError(t, err)
			if len(fr.Check(f)) == 0 {
				continue
			}
			current = string(fr.Fix(f))
		}
		if before == current {
			break
		}
	}
	require.Less(t, passes, maxPasses, "fix did not converge: %q", current)

	// Idempotent: another full pass changes nothing.
	stable := current
	for _, fr := range []interface {
		Check(*lint.File) []lint.Diagnostic
		Fix(*lint.File) []byte
	}{tf, ts} {
		f, err := lint.NewFile("t.md", []byte(stable))
		require.NoError(t, err)
		if len(fr.Check(f)) == 0 {
			continue
		}
		stable = string(fr.Fix(f))
	}
	assert.Equal(t, current, stable, "converged output is not stable")

	// The converged form satisfies MDS060: consistent edge pipes and
	// a blank line on each side of the table (MDS025 owns padding).
	assert.Empty(t, check(t, StyleConsistent, current))
	assert.Contains(t, current, "Text.\n\n|",
		"expected a blank line before the table, got %q", current)
	assert.Contains(t, current, "|\n\nMore text.",
		"expected a blank line after the table, got %q", current)
}
