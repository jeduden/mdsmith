package astutil

import (
	"bytes"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
)

// --- HeadingLine ---

func TestHeadingLine_SetextHeading(t *testing.T) {
	src := []byte("Title\n=====\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	var line int
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			line = HeadingLine(h, f)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	assert.Equal(t, 1, line)
}

func TestHeadingLine_ATXHeading(t *testing.T) {
	src := []byte("# Title\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	var line int
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			line = HeadingLine(h, f)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	assert.Equal(t, 1, line)
}

func TestHeadingLine_ATXOnLaterLine(t *testing.T) {
	src := []byte("Text\n\n## Heading\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	var line int
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			line = HeadingLine(h, f)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	assert.Equal(t, 3, line)
}

func TestHeadingLine_ATXEmphasisOnLaterLine(t *testing.T) {
	// ATX heading on line 3 whose only child is emphasis (not a direct *ast.Text).
	// HeadingLine must descend into inline children to find the text segment.
	src := []byte("Text\n\n## *emph*\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	var line int
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			line = HeadingLine(h, f)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	assert.Equal(t, 3, line)
}

func TestHeadingLine_ATXLinkOnLaterLine(t *testing.T) {
	// ATX heading on line 3 whose only child is a link node.
	src := []byte("Text\n\n## [link](url)\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	var line int
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			line = HeadingLine(h, f)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	assert.Equal(t, 3, line)
}

func TestHeadingLine_Fallback_Returns1(t *testing.T) {
	heading := ast.NewHeading(1)
	f, err := lint.NewFile("test.md", []byte("# X\n"))
	require.NoError(t, err)
	assert.Equal(t, 1, HeadingLine(heading, f))
}

// --- ParagraphLine ---

func TestParagraphLine_FirstLine(t *testing.T) {
	src := []byte("Hello world.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	var line int
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if p, ok := n.(*ast.Paragraph); ok {
			line = ParagraphLine(p, f)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	assert.Equal(t, 1, line)
}

func TestParagraphLine_LaterLine(t *testing.T) {
	src := []byte("# Title\n\nParagraph here.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	var line int
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if p, ok := n.(*ast.Paragraph); ok {
			line = ParagraphLine(p, f)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	assert.Equal(t, 3, line)
}

func TestParagraphLine_Fallback_Returns1(t *testing.T) {
	para := ast.NewParagraph()
	f, err := lint.NewFile("test.md", []byte("text\n"))
	require.NoError(t, err)
	assert.Equal(t, 1, ParagraphLine(para, f))
}

// --- IsTable ---

func TestIsTable_TableParagraph(t *testing.T) {
	// goldmark without table extension parses a table as a paragraph
	src := []byte("| A | B |\n| - | - |\n| 1 | 2 |\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	var found bool
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if p, ok := n.(*ast.Paragraph); ok {
			found = IsTable(p, f)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	assert.True(t, found)
}

func TestIsTable_PlainParagraph(t *testing.T) {
	src := []byte("Just text.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	var found bool
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if p, ok := n.(*ast.Paragraph); ok {
			found = IsTable(p, f)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	assert.False(t, found)
}

func TestIsTable_EmptyParagraph_ReturnsFalse(t *testing.T) {
	para := ast.NewParagraph()
	f, err := lint.NewFile("test.md", []byte("text\n"))
	require.NoError(t, err)
	assert.False(t, IsTable(para, f))
}

// --- HeadingText and ExtractText ---

func TestHeadingText_PlainText(t *testing.T) {
	src := []byte("# Hello World\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			text := HeadingText(h, f.Source)
			assert.Equal(t, "Hello World", text)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

func TestHeadingText_NestedEmphasis(t *testing.T) {
	src := []byte("# Hello *world*\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			text := HeadingText(h, f.Source)
			assert.Equal(t, "Hello world", text)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

func TestExtractText_DirectTextNode(t *testing.T) {
	src := []byte("# Title\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			var buf bytes.Buffer
			for c := h.FirstChild(); c != nil; c = c.NextSibling() {
				ExtractText(c, f.Source, &buf)
			}
			assert.Equal(t, "Title", buf.String())
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}
