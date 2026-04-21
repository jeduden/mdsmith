package markdownflavor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"

	"github.com/jeduden/mdsmith/internal/lint"
)

// TestLineColClampsNegativeOffset exercises the guard that clamps a
// negative offset to 0 so callers that subtract past the start of
// f.Source still get a valid (1, 1) position.
func TestLineColClampsNegativeOffset(t *testing.T) {
	line, col := lineCol([]byte("hello\nworld\n"), -5)
	assert.Equal(t, 1, line)
	assert.Equal(t, 1, col)
}

// TestLineColClampsOversizedOffset exercises the guard that clamps
// an offset past len(source) back to len(source) so callers that
// look one byte past EOF still get a valid position.
func TestLineColClampsOversizedOffset(t *testing.T) {
	src := []byte("hello\nworld\n")
	line, col := lineCol(src, len(src)+10)
	assert.Equal(t, 3, line)
	assert.Equal(t, 1, col)
}

// TestLineStartOfClampsOversizedOffset mirrors the same clamp for
// lineStartOf. An offset past EOF clamps to len(source); for a file
// ending in a newline that puts us one byte past the last newline,
// which is the start of the (empty) line after the document.
func TestLineStartOfClampsOversizedOffset(t *testing.T) {
	src := []byte("hello\nworld\n")
	assert.Equal(t, len(src), lineStartOf(src, len(src)+10))
}

// TestLineStartOfMidLine returns the first byte of the line
// containing the given offset.
func TestLineStartOfMidLine(t *testing.T) {
	src := []byte("hello\nworld\n")
	// Offset 8 sits inside "world" — line start is 6.
	assert.Equal(t, 6, lineStartOf(src, 8))
}

// TestFirstTextStartReturnsNegativeForEmptySubtree covers the
// sentinel return path when no Text node can be found under n.
func TestFirstTextStartReturnsNegativeForEmptySubtree(t *testing.T) {
	// An empty file has no children.
	f := mkFile(t, "\n")
	root := f.AST
	// A *real* ast.Document has no Text descendants, so
	// firstTextStart returns -1 for it.
	assert.Equal(t, -1, firstTextStart(root))
}

// TestFindHeadingIDIgnoresHeadingWithoutAttribute confirms that a
// heading parsed without an `id` attribute short-circuits
// findHeadingID and produces no finding.
func TestFindHeadingIDIgnoresHeadingWithoutAttribute(t *testing.T) {
	// "# Heading" alone: no attribute block, no finding.
	fs := findings(t, "# Heading\n")
	assert.False(t, hasFeature(fs, FeatureHeadingIDs))
}

// TestFindHeadingIDIgnoresAttributesWithoutID covers the second
// guard: the heading has an attribute block but no `id` key.
func TestFindHeadingIDIgnoresAttributesWithoutID(t *testing.T) {
	// Goldmark's attribute parser accepts class-only attribute
	// blocks like `{.highlight}`. Those set Attributes() != nil but
	// no "id" key, so findHeadingID should return ok=false.
	fs := findings(t, "# Heading {.highlight}\n")
	assert.False(t, hasFeature(fs, FeatureHeadingIDs))
}

// TestTaskCheckBoxFindingOrphan exercises the defensive fallback in
// taskCheckBoxFinding when the node has no block ancestor — which
// only happens if the AST was hand-constructed rather than produced
// by goldmark. The fallback returns (1, 1).
func TestTaskCheckBoxFindingOrphan(t *testing.T) {
	f, err := lint.NewFile("t.md", []byte("body\n"))
	require.NoError(t, err)
	orphan := extast.NewTaskCheckBox(true)
	got := taskCheckBoxFinding(f, orphan)
	assert.Equal(t, FeatureTaskLists, got.Feature)
	assert.Equal(t, 1, got.Line)
	assert.Equal(t, 1, got.Column)
}

// TestInlineExtFindingOrphan is the same test for inlineExtFinding.
func TestInlineExtFindingOrphan(t *testing.T) {
	f, err := lint.NewFile("t.md", []byte("body\n"))
	require.NoError(t, err)
	orphan := extast.NewFootnoteLink(7)
	got := inlineExtFinding(f, orphan, FeatureFootnotes)
	assert.Equal(t, FeatureFootnotes, got.Feature)
	assert.Equal(t, 1, got.Line)
	assert.Equal(t, 1, got.Column)
}

// TestFindingFromBlockNoLines covers the `lines == nil || .Len()==0`
// short-circuit: a freshly-constructed block with no Lines appended
// falls back to (1, 1).
func TestFindingFromBlockNoLines(t *testing.T) {
	f, err := lint.NewFile("t.md", []byte("body\n"))
	require.NoError(t, err)
	block := ast.NewParagraph() // no Lines appended
	got := findingFromBlock(f, block, FeatureTables)
	assert.Equal(t, FeatureTables, got.Feature)
	assert.Equal(t, 1, got.Line)
	assert.Equal(t, 1, got.Column)
}

// TestNodeByteRangeClampsNegativeStart covers the clamp in
// nodeByteRange that floors a negative firstTextStart result to 0.
// A FootnoteLink has no children and no source segment, so
// firstTextStart returns -1 and nodeByteRange must floor that.
func TestNodeByteRangeClampsNegativeStart(t *testing.T) {
	n := extast.NewFootnoteLink(7)
	start, end := nodeByteRange(n)
	assert.Equal(t, 0, start)
	assert.Equal(t, 0, end)
}

// TestNearestBlockAncestorSkipsNonBlockAncestors exercises the
// "parent is not a block" branch in nearestBlockAncestor: when we
// walk through an inline ancestor on the way up, the helper skips
// it and keeps climbing.
func TestNearestBlockAncestorSkipsNonBlockAncestors(t *testing.T) {
	// Build: Paragraph (block, has Lines) → Emphasis (inline) →
	// FootnoteLink (inline). Walking up from the FootnoteLink must
	// skip Emphasis and return the Paragraph.
	p := ast.NewParagraph()
	// Append a line so findingFromBlock can resolve a position
	// later (not needed here, but keeps the block well-formed).
	p.Lines().Append(text.NewSegment(0, 1))
	em := ast.NewEmphasis(1)
	link := extast.NewFootnoteLink(1)
	p.AppendChild(p, em)
	em.AppendChild(em, link)

	got := nearestBlockAncestor(link)
	assert.Same(t, ast.Node(p), got)
}

// TestFindHeadingIDHandlesMissingLines exercises the
// "lines == nil || lines.Len() == 0" rejection branch in
// findHeadingID. Normal parsing always fills in Lines on a
// Heading, so we synthesise a Heading with the id attribute set
// but no Lines appended.
func TestFindHeadingIDHandlesMissingLines(t *testing.T) {
	f, err := lint.NewFile("t.md", []byte("# Heading {#top}\n"))
	require.NoError(t, err)
	h := ast.NewHeading(1)
	h.SetAttributeString("id", []byte("top"))
	_, ok := findHeadingID(f, h)
	assert.False(t, ok,
		"findHeadingID must return ok=false when Lines is empty")
}

// TestFindHeadingIDHandlesNoOpeningBrace covers the "brace < 0"
// branch: a Heading whose id attribute was somehow set but whose
// source line contains no `{`. The parser ordinarily does not
// produce such a node; we construct one directly.
func TestFindHeadingIDHandlesNoOpeningBrace(t *testing.T) {
	f, err := lint.NewFile("t.md", []byte("# plain heading\n"))
	require.NoError(t, err)
	h := ast.NewHeading(1)
	h.SetAttributeString("id", []byte("top"))
	h.Lines().Append(text.NewSegment(2, 15))
	_, ok := findHeadingID(f, h)
	assert.False(t, ok,
		"findHeadingID must return ok=false when source line contains no '{'")
}
