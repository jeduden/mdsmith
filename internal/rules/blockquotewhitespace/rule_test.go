package blockquotewhitespace

import (
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

// --- Helper function coverage ---

func TestNodeFirstLine_EmptyNode(t *testing.T) {
	f, err := lint.NewFile("test.md", []byte(""))
	require.NoError(t, err)
	// A node with no lines and no children returns 0.
	n := goldmarkast.NewParagraph()
	assert.Equal(t, 0, nodeFirstLine(f, n))
}

func TestNodeLastLine_EmptyNode(t *testing.T) {
	f, err := lint.NewFile("test.md", []byte(""))
	require.NoError(t, err)
	// A node with no lines and no children returns 0.
	n := goldmarkast.NewParagraph()
	assert.Equal(t, 0, nodeLastLine(f, n))
}

func TestNodeLastLine_ZeroStopSegment(t *testing.T) {
	f, err := lint.NewFile("test.md", []byte("text\n"))
	require.NoError(t, err)
	// When the last segment has Stop=0 the fallback uses Start.
	n := goldmarkast.NewParagraph()
	n.Lines().Append(goldmarktext.NewSegment(0, 0))
	got := nodeLastLine(f, n)
	assert.Equal(t, 1, got)
}

func TestCheck_MD028_EmptyFirstBlockquote_NoFlag(t *testing.T) {
	// A bare > with no content produces an empty blockquote; nodeLastLine
	// returns 0 so the guard fires and nothing is flagged.
	src := []byte(">\n\n> second\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestIsBlankLine_OutOfBounds(t *testing.T) {
	f, err := lint.NewFile("test.md", []byte("> text\n"))
	require.NoError(t, err)
	assert.False(t, isBlankLine(f, 0))   // idx = -1, before start
	assert.False(t, isBlankLine(f, 100)) // idx beyond end of file
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
