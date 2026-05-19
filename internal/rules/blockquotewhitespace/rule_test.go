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

func TestNodeLastLine(t *testing.T) {
	f, err := lint.NewFile("test.md", []byte("line1\nline2\n"))
	require.NoError(t, err)
	t.Run("no_lines_no_children_returns_zero", func(t *testing.T) {
		assert.Equal(t, 0, nodeLastLine(f, goldmarkast.NewParagraph()))
	})
	t.Run("stop_positive_returns_correct_line", func(t *testing.T) {
		n := goldmarkast.NewParagraph()
		n.Lines().Append(goldmarktext.NewSegment(0, 6)) // "line1\n" → stop-1=5 → line 1
		assert.Equal(t, 1, nodeLastLine(f, n))
	})
	t.Run("stop_zero_uses_start", func(t *testing.T) {
		n := goldmarkast.NewParagraph()
		n.Lines().Append(goldmarktext.NewSegment(0, 0)) // stop=0 → falls back to start=0 → line 1
		assert.Equal(t, 1, nodeLastLine(f, n))
	})
	t.Run("recurse_into_children", func(t *testing.T) {
		child := goldmarkast.NewParagraph()
		child.Lines().Append(goldmarktext.NewSegment(6, 12)) // "line2\n" → stop-1=11 → line 2
		parent := goldmarkast.NewBlockquote()
		parent.AppendChild(parent, child)
		assert.Equal(t, 2, nodeLastLine(f, parent))
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

	t.Run("non_blank_gap_not_flagged", func(t *testing.T) {
		// Synthetic AST: two adjacent sibling blockquotes with a non-blank line
		// between their goldmark-reported positions. Exercises the early-return
		// inside the gap loop when isBlankLine returns false.
		//
		// Source layout (bytes):
		//   Line 1 "> bq1\n"   bytes  0-5   (stop=6)
		//   Line 2 "non-blank\n" bytes 6-15
		//   Line 3 "> bq2\n"   bytes 16-21  (start=18 after "> ")
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
		diags := r.checkBlankBetween(f)
		assert.Empty(t, diags)
	})

	t.Run("no_adjacent_blockquotes_no_diags", func(t *testing.T) {
		src := []byte("> only one\n")
		f, err := lint.NewFile("test.md", src)
		require.NoError(t, err)
		assert.Empty(t, r.checkBlankBetween(f))
	})

	t.Run("empty_first_blockquote_guard_fires", func(t *testing.T) {
		// A bare > produces a blockquote whose nodeLastLine returns 0;
		// the guard in checkBlankBetween fires and nothing is flagged.
		src := []byte(">\n\n> second\n")
		f, err := lint.NewFile("test.md", src)
		require.NoError(t, err)
		assert.Empty(t, r.checkBlankBetween(f))
	})
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
