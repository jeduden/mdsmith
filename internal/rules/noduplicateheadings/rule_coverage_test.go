package noduplicateheadings

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
)

// --- Check with setext duplicate headings ---

func TestCheck_SetextDuplicateHeadings(t *testing.T) {
	src := []byte("Title\n=====\n\nTitle\n=====\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "duplicate heading")
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
	v := &visitor{rule: &Rule{}, seen: map[string]int{}}
	assert.Nil(t, v.VisitNode(ast.NewParagraph(), true, &lint.File{}))
}
