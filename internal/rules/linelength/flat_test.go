package linelength

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLineCapable pins the rule.LineCapable marker the engine's parse-skip
// gate keys on: line-length reads only f.Lines and the classifier-backed
// projections, so it always reports true.
func TestLineCapable(t *testing.T) {
	assert.True(t, (&Rule{}).LineCapable())
}

// TestCollectHeadingLines_FlatPath covers the flat-classifier branch of
// collectHeadingLines: on a File built by the parse-skip path (no AST), the
// per-heading-limit line set is served from the classifier — the ATX line,
// and a Setext heading's title line plus its underline.
func TestCollectHeadingLines_FlatPath(t *testing.T) {
	flat, release := lint.NewFileFlatPooled("t.md", []byte("# H1\n\nSetext\n===\n"), false)
	defer release()
	got := collectHeadingLines(flat)
	require.NotNil(t, got)
	for _, ln := range []int{1, 3, 4} {
		_, ok := got[ln]
		assert.Truef(t, ok, "expected heading line %d in the flat set", ln)
	}
}

// TestCollectHeadingLines_ASTPath keeps the AST fallback covered: an
// AST-backed File walks the node tree, yielding the same ATX + Setext set.
func TestCollectHeadingLines_ASTPath(t *testing.T) {
	f, err := lint.NewFile("t.md", []byte("# H1\n\nSetext\n===\n"))
	require.NoError(t, err)
	got := collectHeadingLines(f)
	for _, ln := range []int{1, 3, 4} {
		_, ok := got[ln]
		assert.Truef(t, ok, "expected heading line %d in the AST set", ln)
	}
}
