package blanklinearoundlists

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
)

// --- isInlineNode coverage ---

func TestIsInlineNode_Text(t *testing.T) {
	assert.True(t, isInlineNode(ast.NewText()))
}

func TestIsInlineNode_String(t *testing.T) {
	assert.True(t, isInlineNode(ast.NewString([]byte("hello"))))
}

func TestIsInlineNode_CodeSpan(t *testing.T) {
	assert.True(t, isInlineNode(ast.NewCodeSpan()))
}

func TestIsInlineNode_Emphasis(t *testing.T) {
	assert.True(t, isInlineNode(ast.NewEmphasis(1)))
}

func TestIsInlineNode_Link(t *testing.T) {
	assert.True(t, isInlineNode(ast.NewLink()))
}

func TestIsInlineNode_Image(t *testing.T) {
	assert.True(t, isInlineNode(ast.NewImage(ast.NewLink())))
}

func TestIsInlineNode_AutoLink(t *testing.T) {
	assert.True(t, isInlineNode(ast.NewAutoLink(ast.AutoLinkURL, &ast.Text{})))
}

func TestIsInlineNode_RawHTML(t *testing.T) {
	assert.True(t, isInlineNode(ast.NewRawHTML()))
}

func TestIsInlineNode_Paragraph(t *testing.T) {
	assert.False(t, isInlineNode(ast.NewParagraph()))
}

func TestIsInlineNode_Heading(t *testing.T) {
	assert.False(t, isInlineNode(ast.NewHeading(1)))
}

func TestIsInlineNode_List(t *testing.T) {
	assert.False(t, isInlineNode(ast.NewList('-')))
}

// --- lineOfNode coverage ---

func TestLineOfNode_TextNode(t *testing.T) {
	src := []byte("# Title\n\n- item\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if txt, ok := n.(*ast.Text); ok {
			line := lineOfNode(f, txt)
			assert.Greater(t, line, 0)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

func TestLineOfNode_InlineNode_Emphasis(t *testing.T) {
	src := []byte("- *bold item*\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	found := false
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if _, ok := n.(*ast.Emphasis); ok {
			line := lineOfNode(f, n)
			assert.Equal(t, 1, line)
			found = true
			return ast.WalkContinue, nil
		}
		return ast.WalkContinue, nil
	})
	assert.True(t, found)
}

func TestLineOfNode_BlockNodeWithLines(t *testing.T) {
	src := []byte("paragraph text\n\n- item\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if _, ok := n.(*ast.Paragraph); ok {
			line := lineOfNode(f, n)
			assert.Greater(t, line, 0)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

func TestLineOfNode_ContainerNode(t *testing.T) {
	// List node is a container, resolved via children
	src := []byte("- item 1\n- item 2\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if _, ok := n.(*ast.List); ok {
			line := lineOfNode(f, n)
			assert.Equal(t, 1, line)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

func TestLineOfNode_EmptyNode_Returns0(t *testing.T) {
	node := ast.NewParagraph() // no lines, no children
	f, err := lint.NewFile("test.md", []byte("text\n"))
	require.NoError(t, err)
	line := lineOfNode(f, node)
	assert.Equal(t, 0, line)
}

// --- lastLineOfNode coverage ---

func TestLastLineOfNode_TextNode(t *testing.T) {
	src := []byte("- item\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if txt, ok := n.(*ast.Text); ok {
			line := lastLineOfNode(f, txt)
			assert.Greater(t, line, 0)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

func TestLastLineOfNode_InlineEmphasis(t *testing.T) {
	src := []byte("- *bold*\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	found := false
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if _, ok := n.(*ast.Emphasis); ok {
			line := lastLineOfNode(f, n)
			assert.Equal(t, 1, line)
			found = true
			return ast.WalkContinue, nil
		}
		return ast.WalkContinue, nil
	})
	assert.True(t, found)
}

func TestLastLineOfNode_BlockNodeWithLines(t *testing.T) {
	src := []byte("paragraph text\n\n- item\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if _, ok := n.(*ast.Paragraph); ok {
			line := lastLineOfNode(f, n)
			assert.Greater(t, line, 0)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

func TestLastLineOfNode_ContainerViaChildren(t *testing.T) {
	src := []byte("- item 1\n- item 2\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if _, ok := n.(*ast.List); ok {
			line := lastLineOfNode(f, n)
			assert.Equal(t, 2, line)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

func TestLastLineOfNode_EmptyNode_Returns0(t *testing.T) {
	node := ast.NewParagraph()
	f, err := lint.NewFile("test.md", []byte("text\n"))
	require.NoError(t, err)
	line := lastLineOfNode(f, node)
	assert.Equal(t, 0, line)
}

// --- Category ---

func TestCategory(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "list", r.Category())
}
