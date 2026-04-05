package headingincrement

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
)

// --- headingLine coverage ---

func TestHeadingLine_SetextHeading(t *testing.T) {
	// Setext headings populate Lines(), covering the first branch
	src := []byte("Title\n=====\n\nSection\n-------\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	var lines []int
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			lines = append(lines, headingLine(h, f))
		}
		return ast.WalkContinue, nil
	})
	require.Len(t, lines, 2)
	assert.Equal(t, 1, lines[0])
	assert.Equal(t, 4, lines[1])
}

func TestHeadingLine_ATXHeading(t *testing.T) {
	src := []byte("# Title\n\n## Section\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	var lines []int
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			lines = append(lines, headingLine(h, f))
		}
		return ast.WalkContinue, nil
	})
	require.Len(t, lines, 2)
	assert.Equal(t, 1, lines[0])
	assert.Equal(t, 3, lines[1])
}

func TestHeadingLine_Fallback_Returns1(t *testing.T) {
	heading := ast.NewHeading(1)
	f, err := lint.NewFile("test.md", []byte("# X\n"))
	require.NoError(t, err)
	line := headingLine(heading, f)
	assert.Equal(t, 1, line)
}

// --- Check with setext headings (exercises headingLine's Lines() branch) ---

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
