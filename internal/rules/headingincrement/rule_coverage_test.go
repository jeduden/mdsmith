package headingincrement

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
)

// --- Check with setext headings (exercises astutil.HeadingLine's Lines() branch) ---

func TestCheck_SetextHeadings_ProperIncrement(t *testing.T) {
	src := []byte("Title\n=====\n\nSection\n-------\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	assert.Len(t, diags, 0)
}

func TestCheck_SetextToATX_SkipsLevel(t *testing.T) {
	src := []byte("Title\n=====\n\n### H3\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "incremented from 1 to 3")
}

// --- Category ---

func TestCategory(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "heading", r.Category())
}

// TestVisitNode_NonHeadingReturnsNil drives the defensive type-assert
// guard directly. The shared walk routes only KindHeading to this
// visitor, so the !ok branch is unreachable on the real path, but the
// guard must still return nil (not panic) for any other node kind.
func TestVisitNode_NonHeadingReturnsNil(t *testing.T) {
	v := &visitor{rule: &Rule{}}
	assert.Nil(t, v.VisitNode(ast.NewParagraph(), true, &lint.File{}))
}
