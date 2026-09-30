package refactor

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDestLocator_AutoLinkMovesCursorPastClosingBracket pins that an
// autolink moves the cursor past its closing `>`, for a URL and an
// email autolink alike. Pos()+len(Label) stopped two bytes short.
func TestDestLocator_AutoLinkMovesCursorPastClosingBracket(t *testing.T) {
	body := "<http://h.io> <x@y.io>\n"
	lf, err := lint.NewFile("a.md", []byte(body))
	require.NoError(t, err)
	var ends []int
	_ = ast.Walk(lf.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if al, ok := n.(*ast.AutoLink); ok && entering {
			d := destLocator{lf: lf}
			_, _ = d.visit(al, true)
			ends = append(ends, d.cursor)
		}
		return ast.WalkContinue, nil
	})
	assert.Equal(t, []int{len("<http://h.io>"), len("<http://h.io> <x@y.io>")}, ends)
}

// TestDestLocator_RawHTMLWithoutSegmentsKeepsCursor pins that a raw
// HTML node with no segments neither panics nor moves the cursor.
func TestDestLocator_RawHTMLWithoutSegmentsKeepsCursor(t *testing.T) {
	d := destLocator{cursor: 4}
	assert.NotPanics(t, func() { _, _ = d.visit(ast.NewRawHTML(), true) })
	assert.Equal(t, 4, d.cursor)
}

// TestDestLocator_CursorNeverMovesBack pins the monotonic guard: a
// node that ends, or a link that opens, before the cursor leaves it
// where it is, so an earlier `](` is never reached again.
func TestDestLocator_CursorNeverMovesBack(t *testing.T) {
	d := destLocator{cursor: 9}
	d.advance(true, 3)
	assert.Equal(t, 9, d.cursor)
	d.linkNode(true, 2, nil, true)
	assert.Equal(t, 9, d.cursor)
}
