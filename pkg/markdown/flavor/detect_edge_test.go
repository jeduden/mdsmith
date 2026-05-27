package flavor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// TestLineColClampsNegativeOffset exercises the guard that clamps a
// negative offset to 0 so callers that subtract past the start of
// source still get a valid (1, 1) position.
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
	doc := mkDoc(t, "\n")
	// A *real* ast.Document has no Text descendants, so
	// firstTextStart returns -1 for it.
	assert.Equal(t, -1, firstTextStart(doc.AST))
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
	source := []byte("body\n")
	orphan := extast.NewTaskCheckBox(true)
	got := taskCheckBoxFinding(source, orphan)
	assert.Equal(t, FeatureTaskLists, got.Feature)
	assert.Equal(t, 1, got.Line)
	assert.Equal(t, 1, got.Column)
}

// TestInlineExtFindingOrphan is the same test for inlineExtFinding.
func TestInlineExtFindingOrphan(t *testing.T) {
	source := []byte("body\n")
	orphan := extast.NewFootnoteLink(7)
	got := inlineExtFinding(source, orphan, FeatureFootnotes)
	assert.Equal(t, FeatureFootnotes, got.Feature)
	assert.Equal(t, 1, got.Line)
	assert.Equal(t, 1, got.Column)
}

// TestFindingFromBlockNoLines covers the `lines == nil || .Len()==0`
// short-circuit: a freshly-constructed block with no Lines appended
// falls back to (1, 1).
func TestFindingFromBlockNoLines(t *testing.T) {
	source := []byte("body\n")
	block := ast.NewParagraph() // no Lines appended
	got := findingFromBlock(source, block, FeatureTables)
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

// TestNearestBlockAncestor exercises the "parent is not a block"
// branch in nearestBlockAncestor: when the walk encounters an
// inline ancestor on the way up, the helper skips it and keeps
// climbing. Also covers the nil-parent return when an orphan node
// has no ancestor at all.
func TestNearestBlockAncestor(t *testing.T) {
	t.Run("skips non-block ancestors", func(t *testing.T) {
		// Paragraph (block, has Lines) → Emphasis (inline) →
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
		assert.Same(t, ast.Node(p), nearestBlockAncestor(link))
	})

	t.Run("returns nil for orphan node", func(t *testing.T) {
		assert.Nil(t, nearestBlockAncestor(extast.NewFootnoteLink(1)))
	})
}

// TestNearestBlockAncestorPublic is the dedicated unit test for the
// public NearestBlockAncestor wrapper. The wrapper delegates to the
// unexported helper, so this confirms the surface forwards without
// re-implementing the walk.
func TestNearestBlockAncestorPublic(t *testing.T) {
	p := ast.NewParagraph()
	p.Lines().Append(text.NewSegment(0, 1))
	link := extast.NewFootnoteLink(1)
	p.AppendChild(p, link)
	assert.Same(t, ast.Node(p), NearestBlockAncestor(link))
	assert.Nil(t, NearestBlockAncestor(extast.NewFootnoteLink(1)))
}

// TestIsGitHubAlertPublic exercises the public IsGitHubAlert wrapper
// on both branches of the underlying isGitHubAlert helper: a
// well-formed alert blockquote returns true; a blockquote whose
// first child is not a paragraph returns false.
func TestIsGitHubAlertPublic(t *testing.T) {
	t.Run("recognises alert blockquote", func(t *testing.T) {
		src := []byte("> [!NOTE]\n> body\n")
		root := mkDoc(t, string(src))
		var bq *ast.Blockquote
		_ = ast.Walk(root.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering {
				return ast.WalkContinue, nil
			}
			if b, ok := n.(*ast.Blockquote); ok {
				bq = b
				return ast.WalkStop, nil
			}
			return ast.WalkContinue, nil
		})
		require.NotNil(t, bq, "expected the CommonMark parse to produce a *ast.Blockquote")
		assert.True(t, IsGitHubAlert(bq, src))
	})

	t.Run("rejects non-paragraph first child", func(t *testing.T) {
		// A blockquote whose first child is a heading short-circuits
		// the type assertion inside isGitHubAlert.
		src := []byte("> # heading\n")
		root := mkDoc(t, string(src))
		var bq *ast.Blockquote
		_ = ast.Walk(root.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering {
				return ast.WalkContinue, nil
			}
			if b, ok := n.(*ast.Blockquote); ok {
				bq = b
				return ast.WalkStop, nil
			}
			return ast.WalkContinue, nil
		})
		require.NotNil(t, bq)
		assert.False(t, IsGitHubAlert(bq, src))
	})
}

// TestLineColPublic is the dedicated unit test for the public
// LineCol wrapper. The contract test exercises the call shape; this
// test pins the documented "1-based" semantics on real input.
func TestLineColPublic(t *testing.T) {
	src := []byte("hello\nworld\n")
	line, col := LineCol(src, 6) // start of "world"
	assert.Equal(t, 2, line)
	assert.Equal(t, 1, col)
}

// TestDualFindings exercises the pooled-parser helper extracted from
// Detect. The accept predicate must filter findings at the helper's
// own seam — a keep callback that rejects FeatureTables must not
// emit a Tables finding even though the dual AST contains a Table.
func TestDualFindings(t *testing.T) {
	src := []byte("| a | b |\n| - | - |\n| 1 | 2 |\n\n~~old~~\n")
	rejectTables := func(feat Feature) bool {
		return feat != FeatureTables
	}
	got := dualFindings(src, rejectTables)
	for _, f := range got {
		assert.NotEqual(t, FeatureTables, f.Feature,
			"keep predicate must suppress FeatureTables findings")
	}
	// Strikethrough is still kept, so the helper still does real work.
	found := false
	for _, f := range got {
		if f.Feature == FeatureStrikethrough {
			found = true
		}
	}
	assert.True(t, found, "expected at least one Strikethrough finding")
}

// TestFindHeadingIDHandlesMissingLines exercises the
// "lines == nil || lines.Len() == 0" rejection branch in
// findHeadingID. Normal parsing always fills in Lines on a
// Heading, so we synthesise a Heading with the id attribute set
// but no Lines appended.
func TestFindHeadingIDHandlesMissingLines(t *testing.T) {
	h := ast.NewHeading(1)
	h.SetAttributeString("id", []byte("top"))
	_, ok := findHeadingID([]byte("# Heading {#top}\n"), h)
	assert.False(t, ok,
		"findHeadingID must return ok=false when Lines is empty")
}

// TestFindHeadingIDHandlesNoOpeningBrace covers the "brace < 0"
// branch: a Heading whose id attribute was somehow set but whose
// source line contains no `{`. The parser ordinarily does not
// produce such a node; we construct one directly.
func TestFindHeadingIDHandlesNoOpeningBrace(t *testing.T) {
	h := ast.NewHeading(1)
	h.SetAttributeString("id", []byte("top"))
	h.Lines().Append(text.NewSegment(2, 15))
	_, ok := findHeadingID([]byte("# plain heading\n"), h)
	assert.False(t, ok,
		"findHeadingID must return ok=false when source line contains no '{'")
}

// TestFindHeadingIDPublicReturnsExtra exercises the success path of
// the public FindHeadingID wrapper: a real heading with {#id} parsed
// by the dual parser must round-trip through the wrapper with the
// attribute byte span exposed via HeadingIDExtra.
func TestFindHeadingIDPublicReturnsExtra(t *testing.T) {
	src := []byte("# Heading {#custom}\n")
	root := parseSource(t, src)
	var found *ast.Heading
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			found = h
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	require.NotNil(t, found, "expected the parser to produce a *ast.Heading")
	hx, ok := FindHeadingID(src, found)
	require.True(t, ok, "expected FindHeadingID to locate the {#custom} attr block")
	// The span should cover exactly "{#custom}".
	assert.Equal(t, "{#custom}", string(src[hx.AttrStart:hx.AttrEnd]))
}

// TestFindHeadingIDPublicWrapsMissID exercises the !ok branch of
// FindHeadingID: a heading without the `id` attribute returns the
// zero HeadingIDExtra and ok=false. The wrapper must not panic on
// the missing assertion.
func TestFindHeadingIDPublicWrapsMissID(t *testing.T) {
	h := ast.NewHeading(1)
	hx, ok := FindHeadingID([]byte("# plain\n"), h)
	assert.False(t, ok)
	assert.Equal(t, HeadingIDExtra{}, hx)
}
