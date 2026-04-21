package markdownflavor

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
