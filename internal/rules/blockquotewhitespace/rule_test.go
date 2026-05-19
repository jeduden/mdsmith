package blockquotewhitespace

import (
	"bytes"
	"testing"

	goldmarkast "github.com/yuin/goldmark/ast"
	goldmarktext "github.com/yuin/goldmark/text"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- MD027: multiple spaces after blockquote marker ---

func TestCheck_MD027_TwoSpaces(t *testing.T) {
	src := []byte(">  quoted text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, 1, diags[0].Line)
	assert.Equal(t, 1, diags[0].Column)
	assert.Equal(t, "multiple spaces after blockquote marker", diags[0].Message)
	assert.Equal(t, "MDS059", diags[0].RuleID)
}

func TestCheck_MD027_ThreeSpaces(t *testing.T) {
	src := []byte(">   three spaces\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, 1, diags[0].Line)
}

func TestCheck_MD027_OneSpace_Clean(t *testing.T) {
	src := []byte("> single space\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_MD027_NoSpace_Clean(t *testing.T) {
	src := []byte(">no space\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_MD027_NestedBlockquote(t *testing.T) {
	// Nested blockquote: inner > also has multiple spaces
	src := []byte("> >  nested\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, 1, diags[0].Line)
	assert.Equal(t, 3, diags[0].Column) // ">  " starts at byte index 2, column 3
}

func TestCheck_MD027_ContentArrow_NoFlag(t *testing.T) {
	// A > inside blockquote content (not the marker) must not be flagged.
	src := []byte("> text >  more\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_MD027_DeepListItemBlockquote(t *testing.T) {
	// Blockquote inside a nested list item can have 4+ spaces of indent in the
	// raw source; the prefix regex must capture the > at that depth.
	src := []byte("- outer\n  - inner\n    >  text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, 3, diags[0].Line)
}

func TestFix_MD027_DeepListItemBlockquote(t *testing.T) {
	src := []byte("- outer\n  - inner\n    >  text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	got := r.Fix(f)
	assert.Equal(t, "- outer\n  - inner\n    > text\n", string(got))
}

func TestCheck_MD027_SkipsFencedCodeBlock(t *testing.T) {
	src := []byte("```\n>  not flagged inside code\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_MD027_MultipleViolationsOnDifferentLines(t *testing.T) {
	src := []byte(">  first\n>  second\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 2)
	assert.Equal(t, 1, diags[0].Line)
	assert.Equal(t, 2, diags[1].Line)
}

// --- MD028: blank line between blockquotes ---

func TestCheck_MD028_BlankBetweenBlockquotes(t *testing.T) {
	src := []byte("# Title\n\n> first\n\n> second\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "blank line between blockquotes", diags[0].Message)
	assert.Equal(t, "MDS059", diags[0].RuleID)
	assert.Equal(t, 1, diags[0].Column)
}

func TestCheck_MD028_NoBlankBetween_Clean(t *testing.T) {
	// Two blockquotes with non-blank content between them: no flag.
	src := []byte("> first\n\nsome text\n\n> second\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_MD028_InternalBlankViaMarker_Clean(t *testing.T) {
	// Single blockquote with internal blank lines using > marker: not flagged.
	src := []byte("> first paragraph\n>\n> second paragraph\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_MD028_EmptyFirstBlockquote(t *testing.T) {
	// A bare > (empty first blockquote) followed by a blank line and a second
	// blockquote must be flagged. The backwards-scan approach handles this
	// because it only needs nodeFirstLine of the second blockquote.
	src := []byte(">\n\n> second\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "blank line between blockquotes", diags[0].Message)
}

func TestCheck_MD028_EmptyFile_Clean(t *testing.T) {
	src := []byte("")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_MD028_SingleBlockquote_Clean(t *testing.T) {
	src := []byte("> just one\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

// --- Fix: MD027 autofix ---

func TestFix_MD027_CollapsesMultipleSpaces(t *testing.T) {
	src := []byte(">  quoted\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	got := r.Fix(f)
	assert.Equal(t, "> quoted\n", string(got))
}

func TestFix_MD027_CollapsesThreeSpaces(t *testing.T) {
	src := []byte(">   three\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	got := r.Fix(f)
	assert.Equal(t, "> three\n", string(got))
}

func TestFix_MD027_PreservesSingleSpace(t *testing.T) {
	src := []byte("> single\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	got := r.Fix(f)
	assert.Equal(t, "> single\n", string(got))
}

func TestFix_MD027_SkipsFencedCodeBlock(t *testing.T) {
	src := []byte("```\n>  code block\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	got := r.Fix(f)
	assert.Equal(t, string(src), string(got))
}

func TestFix_MD027_MarkerOnlyLine_NoTrailingSpace(t *testing.T) {
	// ">  " (no content after marker) must be fixed to ">" not "> " to
	// avoid introducing trailing whitespace that would require another pass.
	src := []byte(">  \n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	got := (&Rule{}).Fix(f)
	assert.Equal(t, ">\n", string(got))
}

func TestFix_MD027_NestedMarkerOnlyLine_NoTrailingSpace(t *testing.T) {
	// ">>  " (nested, no content) must be fixed to ">>" not ">> ".
	src := []byte(">>  \n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	got := (&Rule{}).Fix(f)
	assert.Equal(t, ">>\n", string(got))
}

func TestFix_MD028_NoAutoFix(t *testing.T) {
	// MD028 violations are not auto-fixed.
	src := []byte("> first\n\n> second\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	got := r.Fix(f)
	// Fix only touches MD027; MD028 content is preserved.
	assert.Equal(t, string(src), string(got))
}

// --- Helper function tests ---

func TestIsBlankLine(t *testing.T) {
	f, err := lint.NewFile("test.md", []byte("> text\n\n> more\n"))
	require.NoError(t, err)
	t.Run("blank_line_returns_true", func(t *testing.T) {
		assert.True(t, isBlankLine(f, 2))
	})
	t.Run("non_blank_line_returns_false", func(t *testing.T) {
		assert.False(t, isBlankLine(f, 1))
	})
	t.Run("linenum_zero_returns_false", func(t *testing.T) {
		assert.False(t, isBlankLine(f, 0)) // idx = -1
	})
	t.Run("linenum_past_end_returns_false", func(t *testing.T) {
		assert.False(t, isBlankLine(f, 100))
	})
}

func TestFirstBQLineFrom(t *testing.T) {
	f, err := lint.NewFile("test.md", []byte("> a\ntext\n> b\n"))
	require.NoError(t, err)
	t.Run("finds_first_bq", func(t *testing.T) {
		assert.Equal(t, 1, firstBQLineFrom(f, 1))
	})
	t.Run("finds_from_mid_file", func(t *testing.T) {
		assert.Equal(t, 3, firstBQLineFrom(f, 2))
	})
	t.Run("returns_zero_when_not_found", func(t *testing.T) {
		// fromLine past the last BQ line → no match → 0.
		assert.Equal(t, 0, firstBQLineFrom(f, 4))
	})
}

func TestFirstBQLineAfterGap(t *testing.T) {
	t.Run("returns_zero_when_no_blank_in_file", func(t *testing.T) {
		// Non-blank lines fill the whole file: no blank gap exists so the
		// non-blank scan reaches EOF and the first guard fires.
		f := &lint.File{
			Path:  "test.md",
			Lines: [][]byte{[]byte("> bq")},
		}
		assert.Equal(t, 0, firstBQLineAfterGap(f, 1))
	})
	t.Run("returns_zero_when_file_ends_in_blanks", func(t *testing.T) {
		// Non-blank line then two blank lines at EOF: no BQ after the gap.
		f, err := lint.NewFile("test.md", []byte("> bq\n\n\n"))
		require.NoError(t, err)
		assert.Equal(t, 0, firstBQLineAfterGap(f, 1))
	})
	t.Run("returns_zero_when_non_bq_after_gap", func(t *testing.T) {
		// Content after the blank gap is plain text, not a BQ marker.
		f, err := lint.NewFile("test.md", []byte("> bq\n\ntext\n"))
		require.NoError(t, err)
		assert.Equal(t, 0, firstBQLineAfterGap(f, 1))
	})
	t.Run("finds_bq_after_gap", func(t *testing.T) {
		f, err := lint.NewFile("test.md", []byte("> bq1\n\n> bq2\n"))
		require.NoError(t, err)
		assert.Equal(t, 3, firstBQLineAfterGap(f, 1))
	})
}

func TestEmptyBQLine_CannotDeterminePosition(t *testing.T) {
	// Synthetic: two empty sibling blockquotes, but the source has no > markers,
	// so firstBQLineFrom returns 0 and emptyBQLine cannot determine the position.
	// This also exercises the second first==0 guard in checkBlankBetween.
	src := []byte("no blockquotes here\n")
	bq1 := goldmarkast.NewBlockquote()
	bq2 := goldmarkast.NewBlockquote()
	doc := goldmarkast.NewDocument()
	doc.AppendChild(doc, bq1)
	doc.AppendChild(doc, bq2)
	f := &lint.File{
		Path:   "test.md",
		Source: src,
		Lines:  bytes.Split(src, []byte("\n")),
		AST:    doc,
	}
	assert.Empty(t, (&Rule{}).checkBlankBetween(f))
}

func TestEmptyBQLine_GapScanFails(t *testing.T) {
	// Synthetic: positioned bq1 at line 1, empty bq2; source has no blank line
	// between them, so firstBQLineAfterGap returns 0 and the violation is skipped.
	src := []byte("> bq1\n> bq2\n")
	bq1Para := goldmarkast.NewParagraph()
	bq1Para.Lines().Append(goldmarktext.NewSegment(2, 6)) // line 1
	bq1 := goldmarkast.NewBlockquote()
	bq1.AppendChild(bq1, bq1Para)
	bq2 := goldmarkast.NewBlockquote() // empty, no children
	doc := goldmarkast.NewDocument()
	doc.AppendChild(doc, bq1)
	doc.AppendChild(doc, bq2)
	f := &lint.File{
		Path:   "test.md",
		Source: src,
		Lines:  bytes.Split(src, []byte("\n")),
		AST:    doc,
	}
	assert.Empty(t, (&Rule{}).checkBlankBetween(f))
}

func TestEmptyBQLine_ParentHasPosition(t *testing.T) {
	// n has no preceding siblings, but its parent node has a known position
	// (line 1). emptyBQLine uses that as base and finds the BQ marker at line 1.
	src := []byte("> bq\n")
	parent := goldmarkast.NewParagraph()
	parent.Lines().Append(goldmarktext.NewSegment(0, 5)) // nodeFirstLine(parent) = 1
	emptyBQ := goldmarkast.NewBlockquote()               // first child, no siblings
	parent.AppendChild(parent, emptyBQ)
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	assert.Equal(t, 1, emptyBQLine(f, emptyBQ))
}

func TestNodeFirstLine(t *testing.T) {
	f, err := lint.NewFile("test.md", []byte("line1\nline2\n"))
	require.NoError(t, err)
	t.Run("no_lines_no_children_returns_zero", func(t *testing.T) {
		assert.Equal(t, 0, nodeFirstLine(f, goldmarkast.NewParagraph()))
	})
	t.Run("has_lines_returns_first_line", func(t *testing.T) {
		n := goldmarkast.NewParagraph()
		n.Lines().Append(goldmarktext.NewSegment(6, 12)) // "line2\n" → start=6 → line 2
		assert.Equal(t, 2, nodeFirstLine(f, n))
	})
	t.Run("recurse_into_children", func(t *testing.T) {
		child := goldmarkast.NewParagraph()
		child.Lines().Append(goldmarktext.NewSegment(0, 6)) // "line1\n" → start=0 → line 1
		parent := goldmarkast.NewBlockquote()
		parent.AppendChild(parent, child)
		assert.Equal(t, 1, nodeFirstLine(f, parent))
	})
}

func TestRule_checkBlankBetween(t *testing.T) {
	r := &Rule{}

	t.Run("blank_gap_flagged", func(t *testing.T) {
		src := []byte("# Title\n\n> first\n\n> second\n")
		f, err := lint.NewFile("test.md", src)
		require.NoError(t, err)
		diags := r.checkBlankBetween(f)
		require.Len(t, diags, 1)
		assert.Equal(t, "blank line between blockquotes", diags[0].Message)
	})

	t.Run("no_adjacent_blockquotes_no_diags", func(t *testing.T) {
		src := []byte("> only one\n")
		f, err := lint.NewFile("test.md", src)
		require.NoError(t, err)
		assert.Empty(t, r.checkBlankBetween(f))
	})

	t.Run("empty_first_blockquote_flagged", func(t *testing.T) {
		// A bare > produces an empty first blockquote; nodeLastLine would return 0
		// for it, but the backwards-scan only needs nodeFirstLine(nextBq).
		src := []byte(">\n\n> second\n")
		f, err := lint.NewFile("test.md", src)
		require.NoError(t, err)
		diags := r.checkBlankBetween(f)
		require.Len(t, diags, 1)
		assert.Equal(t, "blank line between blockquotes", diags[0].Message)
	})
}

func TestRule_checkBlankBetween_NonBlankGap(t *testing.T) {
	// Synthetic AST: two adjacent sibling blockquotes with a non-blank line
	// between their goldmark-reported positions. Exercises the scanLine>=first-1
	// guard when the line immediately before nextBq is non-blank.
	//
	// Source layout (bytes):
	//   Line 1 "> bq1\n"     bytes  0-5  (stop=6)
	//   Line 2 "non-blank\n" bytes  6-15
	//   Line 3 "> bq2\n"     bytes 16-21 (start=18 after "> ")
	src := []byte("> bq1\nnon-blank\n> bq2\n")

	bq1Para := goldmarkast.NewParagraph()
	bq1Para.Lines().Append(goldmarktext.NewSegment(2, 6)) // stop-1=5 → line 1
	bq1 := goldmarkast.NewBlockquote()
	bq1.AppendChild(bq1, bq1Para)

	bq2Para := goldmarkast.NewParagraph()
	bq2Para.Lines().Append(goldmarktext.NewSegment(18, 22)) // start=18 → line 3
	bq2 := goldmarkast.NewBlockquote()
	bq2.AppendChild(bq2, bq2Para)

	doc := goldmarkast.NewDocument()
	doc.AppendChild(doc, bq1)
	doc.AppendChild(doc, bq2)

	f := &lint.File{
		Path:   "test.md",
		Source: src,
		Lines:  bytes.Split(src, []byte("\n")),
		AST:    doc,
	}
	assert.Empty(t, (&Rule{}).checkBlankBetween(f))
}

func TestRule_checkBlankBetween_NoSegments(t *testing.T) {
	// Synthetic: nextBq has no children with segments, so nodeFirstLine
	// returns 0. The emptyBQLine fallback locates bq2 at line 3 via the
	// source scan, so the blank-line gap is still flagged.
	src := []byte("> bq1\n\n> bq2\n")
	bq1Para := goldmarkast.NewParagraph()
	bq1Para.Lines().Append(goldmarktext.NewSegment(2, 6)) // line 1
	bq1 := goldmarkast.NewBlockquote()
	bq1.AppendChild(bq1, bq1Para)
	// bq2 has no children, so nodeFirstLine returns 0; fallback kicks in.
	bq2 := goldmarkast.NewBlockquote()
	doc := goldmarkast.NewDocument()
	doc.AppendChild(doc, bq1)
	doc.AppendChild(doc, bq2)
	f := &lint.File{
		Path:   "test.md",
		Source: src,
		Lines:  bytes.Split(src, []byte("\n")),
		AST:    doc,
	}
	diags := (&Rule{}).checkBlankBetween(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "blank line between blockquotes", diags[0].Message)
}

func TestCheck_MD028_EmptySecondBlockquote(t *testing.T) {
	// Non-empty first blockquote, empty second blockquote: the blank line
	// between them must still be flagged even though nodeFirstLine(nextBq)==0.
	src := []byte("> text\n\n>\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	diags := (&Rule{}).checkBlankBetween(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "blank line between blockquotes", diags[0].Message)
}

func TestCheck_MD028_BothEmptyBlockquotes(t *testing.T) {
	// Both blockquotes are empty (bare >); the blank line between them must
	// be flagged even though nodeFirstLine returns 0 for both.
	src := []byte(">\n\n>\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	diags := (&Rule{}).checkBlankBetween(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "blank line between blockquotes", diags[0].Message)
}

func TestRule_checkBlankBetween_NextBqAtLineOne(t *testing.T) {
	// Synthetic: nextBq starts at line 1, so first-1==0 and the scanLine<=0
	// guard fires. Nothing before line 1 can be a blank gap.
	src := []byte("> bq\n")
	bq1 := goldmarkast.NewBlockquote()
	bq2Para := goldmarkast.NewParagraph()
	bq2Para.Lines().Append(goldmarktext.NewSegment(0, 4)) // start=0 → line 1
	bq2 := goldmarkast.NewBlockquote()
	bq2.AppendChild(bq2, bq2Para)
	doc := goldmarkast.NewDocument()
	doc.AppendChild(doc, bq1)
	doc.AppendChild(doc, bq2)
	f := &lint.File{
		Path:   "test.md",
		Source: src,
		Lines:  bytes.Split(src, []byte("\n")),
		AST:    doc,
	}
	assert.Empty(t, (&Rule{}).checkBlankBetween(f))
}

// --- Meta ---

func TestRuleID(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "MDS059", r.ID())
}

func TestRuleName(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "blockquote-whitespace", r.Name())
}

func TestCategory(t *testing.T) {
	r := &Rule{}
	assert.NotEmpty(t, r.Category())
}
