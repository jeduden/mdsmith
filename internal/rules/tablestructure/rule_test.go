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

// --- blockquote tables ---

func TestBlockquoteTableClean(t *testing.T) {
	src := "# T\n\n> Intro.\n>\n> | A | B |\n> | - | - |\n> | 1 | 2 |\n>\n> Outro.\n"
	assert.Empty(t, check(t, StyleConsistent, src))
}

func TestBlockquoteMD058FlaggedAndFixed(t *testing.T) {
	src := "# T\n\n> Intro.\n> | A | B |\n> | - | - |\n> Outro.\n"
	diags := check(t, StyleConsistent, src)
	require.Len(t, diags, 2)
	assert.Equal(t, 4, diags[0].Line)
	assert.Equal(t, "missing blank line before table", diags[0].Message)
	assert.Equal(t, 5, diags[1].Line)
	assert.Equal(t, "missing blank line after table", diags[1].Message)

	got := fix(t, StyleConsistent, src)
	want := "# T\n\n> Intro.\n>\n> | A | B |\n> | - | - |\n>\n> Outro.\n"
	assert.Equal(t, want, got)
	assert.Empty(t, check(t, StyleConsistent, got))
}

func TestBlockquoteMixedPipesFixed(t *testing.T) {
	// Borderless header inside a blockquote; consistent expects no
	// edge pipes, so the bordered row is normalized.
	src := "# T\n\n> A | B\n> - | -\n> | 1 | 2 |\n"
	diags := check(t, StyleConsistent, src)
	require.Len(t, diags, 1)
	assert.Equal(t, 5, diags[0].Line)

	got := fix(t, StyleConsistent, src)
	assert.Equal(t, "# T\n\n> A | B\n> - | -\n> 1 | 2\n", got)
}

func TestNestedBlockquoteTable(t *testing.T) {
	src := "# T\n\n> > | A | B |\n> > | - | - |\n> > | 1 | 2 |\n"
	assert.Empty(t, check(t, StyleConsistent, src))
}

func TestBlockquoteCRLFBlankInsertion(t *testing.T) {
	src := "# T\r\n\r\n> Intro.\r\n> | A | B |\r\n> | - | - |\r\n"
	got := fix(t, StyleConsistent, src)
	assert.Contains(t, got, "> Intro.\r\n>\r\n> | A | B |")
	assert.NotContains(t, got, "\r\n\n")
}

func TestBlockquoteNoSpaceNotDetected(t *testing.T) {
	// `>|` (no space after the marker) is not treated as a blockquote
	// table, matching tablefmt; nothing is flagged.
	src := "# T\n\n>| A | B |\n>| - | - |\n"
	assert.Empty(t, check(t, StyleConsistent, src))
}

// --- pipe-style describeStyle branches ---

func TestConsistentLeadingPipeOnly(t *testing.T) {
	// Header has a leading pipe but no trailing pipe; consistent
	// holds the data row to "leading pipe only".
	src := "# T\n\n| A | B\n| - | -\n| 1 | 2 |\n"
	diags := check(t, StyleConsistent, src)
	require.Len(t, diags, 1)
	assert.Equal(t, 5, diags[0].Line)
	assert.Equal(t,
		"table pipe style; expected leading pipe only", diags[0].Message)
}

func TestConsistentTrailingPipeOnly(t *testing.T) {
	src := "# T\n\nA | B |\n- | - |\n| 1 | 2 |\n"
	diags := check(t, StyleConsistent, src)
	require.Len(t, diags, 1)
	assert.Equal(t,
		"table pipe style; expected trailing pipe only", diags[0].Message)
}

// --- cell-count edge cases ---

func TestCountCellsDegenerate(t *testing.T) {
	assert.Equal(t, 0, countCells(""))
	assert.Equal(t, 0, countCells("|"))
	assert.Equal(t, 1, countCells("|  |"))
	assert.Equal(t, 2, countCells("a | b"))
}

func TestEscapedPipeIsOneCell(t *testing.T) {
	// `a \| b` is a single cell; the escaped pipe must not split it.
	src := "# T\n\n| A      | B |\n| ------ | - |\n| a \\| b | c |\n"
	assert.Empty(t, check(t, StyleConsistent, src),
		"escaped pipe must not be counted as a column separator")
}

func TestSeparatorOnlyRowNotHeader(t *testing.T) {
	// Two separator-looking lines: the first cannot be a header.
	src := "# T\n\n| - | - |\n| - | - |\n"
	assert.Empty(t, check(t, StyleConsistent, src))
}

func TestHeadingLineNotTableHeader(t *testing.T) {
	src := "# Title | x\n| - | - |\n"
	assert.Empty(t, check(t, StyleConsistent, src))
}

// --- skip: PI and generated ranges ---

func TestSkipsProcessingInstruction(t *testing.T) {
	src := "# T\n\n<?toc\nmin-level: 2\n?>\n<?/toc?>\n"
	assert.Empty(t, check(t, StyleConsistent, src))
}

func TestSkipsGeneratedRange(t *testing.T) {
	src := "# T\n\nText.\n| A | B |\n| - | - |\nMore.\n"
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	f.GeneratedRanges = []lint.LineRange{{From: 3, To: 6}}
	r := &Rule{Style: StyleConsistent}
	assert.Empty(t, r.Check(f), "tables inside a generated range are skipped")
}

func TestFixNoTablesReturnsCopy(t *testing.T) {
	src := "# T\n\nNo tables here.\n"
	assert.Equal(t, src, fix(t, StyleConsistent, src))
}

func TestBlankLineForAndIsBlankAround(t *testing.T) {
	assert.Equal(t, "", blankLineFor("  "))
	assert.Equal(t, ">", blankLineFor("> "))
	assert.Equal(t, "> >", blankLineFor("> > "))
	assert.True(t, isBlankAround([]byte("   "), ""))
	assert.True(t, isBlankAround([]byte("> >"), "> "))
	assert.False(t, isBlankAround([]byte("> text"), "> "))
	assert.False(t, isBlankAround([]byte("text"), ""))
}

func TestCRLFMixedPipesNormalized(t *testing.T) {
	// Edge normalization on a CRLF file must keep the carriage return.
	src := "# T\r\n\r\nA | B\r\n- | -\r\n| 1 | 2 |\r\n"
	got := fix(t, StyleConsistent, src)
	assert.Equal(t, "# T\r\n\r\nA | B\r\n- | -\r\n1 | 2\r\n", got)
}

func TestSameLinePipeAndColumnDiagnostics(t *testing.T) {
	// Row 5 is bordered (pipe-style mismatch under consistent) and
	// has three cells (column-count mismatch): two diagnostics share
	// the line, exercising the stable sort.
	src := "# T\n\nA | B\n- | -\n| 1 | 2 | 3 |\n"
	diags := check(t, StyleConsistent, src)
	require.Len(t, diags, 2)
	assert.Equal(t, 5, diags[0].Line)
	assert.Equal(t, 5, diags[1].Line)
	msgs := []string{diags[0].Message, diags[1].Message}
	assert.Contains(t, msgs, "table pipe style; expected no leading or trailing pipes")
	assert.Contains(t, msgs, "table column count; expected 2, got 3")
}

func TestIsSeparatorContentDegenerate(t *testing.T) {
	assert.False(t, isSeparatorContent("|"))
	assert.False(t, isSeparatorContent("- | x"))
	assert.True(t, isSeparatorContent(":-: | ---"))
}

func TestDetectPrefixIndentedBlockquote(t *testing.T) {
	assert.Equal(t, "  > ", detectPrefix([]byte("  > | a |")))
	assert.Equal(t, ">", detectPrefix([]byte(">")))
	assert.Equal(t, "\t", detectPrefix([]byte("\t| a |")))
	assert.Equal(t, "", detectPrefix([]byte("| a |")))
}

func TestTrailingEscapedPipeIsNotEdge(t *testing.T) {
	// The final `\|` is a literal pipe in the last cell, not a
	// trailing edge pipe: no false MD055/MD056, two cells.
	src := "# T\n\nA | B\n- | -\na | b \\|\n"
	assert.Empty(t, check(t, StyleConsistent, src),
		"escaped trailing pipe must not be read as a table edge")
}

func TestFixPreservesEscapedTrailingPipe(t *testing.T) {
	// Adding edges must keep the literal `\|`, not strip it.
	src := "# T\n\nA | B\n- | -\na | b \\|\n"
	got := fix(t, StyleLeadingAndTrailing, src)
	want := "# T\n\n| A | B |\n| - | - |\n| a | b \\| |\n"
	assert.Equal(t, want, got)
	assert.Empty(t, check(t, StyleLeadingAndTrailing, got))
}

func TestEndsWithUnescapedPipe(t *testing.T) {
	assert.True(t, endsWithUnescapedPipe("a|"))
	assert.False(t, endsWithUnescapedPipe("a\\|"))
	assert.True(t, endsWithUnescapedPipe("a\\\\|"))
	assert.False(t, endsWithUnescapedPipe("a"))
	assert.False(t, endsWithUnescapedPipe(""))
}

// TestNoLeadingOrTrailingStableWithMDS025 backs the README claim that
// no_leading_or_trailing does not oscillate with MDS025: once MDS060
// strips the edge pipes, MDS025 (which formats only bordered tables)
// stops touching the table, so the loop converges.
func TestNoLeadingOrTrailingStableWithMDS025(t *testing.T) {
	src := "# T\n\n| A | B |\n| - | - |\n| 1 | 2 |\n"
	tf := &tableformat.Rule{Pad: 1}
	ts := &Rule{Style: StyleNoLeadingOrTrailing}

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
	require.Less(t, passes, maxPasses, "did not converge: %q", current)
	assert.Empty(t, check(t, StyleNoLeadingOrTrailing, current))
	assert.NotContains(t, current, "|\n", "table should be borderless")
}

func TestContainsUnescapedPipe(t *testing.T) {
	assert.True(t, containsUnescapedPipe("a|b"))
	assert.False(t, containsUnescapedPipe("a\\|b"))
	assert.True(t, containsUnescapedPipe("a\\\\|b"))
	assert.True(t, containsUnescapedPipe("a\\|b|c"))
	assert.False(t, containsUnescapedPipe("plain text"))
}

func TestSplitCellsBackslashParity(t *testing.T) {
	// `\\|` is "escaped backslash" followed by an unescaped pipe
	// delimiter — two cells, not one.
	assert.Equal(t, []string{"\\\\", ""}, splitCells("\\\\|"))
	// `\\\|` is "escaped backslash" then "escaped pipe" — one cell.
	assert.Equal(t, []string{"\\\\\\|"}, splitCells("\\\\\\|"))
}

func TestEscapedPipeOnlyParagraphNotHeader(t *testing.T) {
	// "A \| B" contains only an escaped pipe; even when followed by a
	// delimiter-looking row, it is not a table header.
	src := "# T\n\nA \\| B\n--- | ---\n1 | 2\n"
	assert.Empty(t, check(t, StyleConsistent, src))
}

func TestEscapedPipeParagraphAfterTableEndsIt(t *testing.T) {
	// A paragraph whose only pipe is escaped must not be absorbed as
	// a body row; MD058 must still flag the missing blank after.
	src := "# T\n\n| A | B |\n| - | - |\n| 1 | 2 |\nText \\| more.\n"
	diags := check(t, StyleConsistent, src)
	require.Len(t, diags, 1)
	assert.Equal(t, "missing blank line after table", diags[0].Message)
	assert.Equal(t, 5, diags[0].Line)
}

func TestHashStartingCellNotMistakenForHeading(t *testing.T) {
	// `#1` (hash directly followed by a non-space) is not an ATX
	// heading — it's a valid first cell, so the table must still
	// be detected and clean.
	src := "# T\n\n#1 | Title\n--- | -----\nA | B\n"
	assert.Empty(t, check(t, StyleConsistent, src))
}

func TestIsATXHeading(t *testing.T) {
	assert.True(t, isATXHeading("# Title"))
	assert.True(t, isATXHeading("###### Six"))
	assert.True(t, isATXHeading("##")) // empty heading
	assert.False(t, isATXHeading("#1 | Title"))
	assert.False(t, isATXHeading("####### Seven")) // >6 hashes
	assert.False(t, isATXHeading("text"))
}

func TestParseRowIgnoresPostPrefixIndent(t *testing.T) {
	// Extra spaces after the blockquote marker should not break
	// leading-pipe detection: the table is valid and clean.
	src := "# T\n\n> Intro.\n>\n>  | A | B |\n>  | - | - |\n>  | 1 | 2 |\n>\n> Outro.\n"
	assert.Empty(t, check(t, StyleConsistent, src))
}
