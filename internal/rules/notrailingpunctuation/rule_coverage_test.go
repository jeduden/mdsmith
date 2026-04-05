package notrailingpunctuation

import (
	"bytes"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
)

// --- headingLine coverage ---

func TestHeadingLine_SetextHeading(t *testing.T) {
	// Setext headings have Lines(), so the first branch of headingLine is taken
	src := []byte("Title\n=====\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	var line int
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			line = headingLine(h, f)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	assert.Equal(t, 1, line)
}

func TestHeadingLine_ATXHeading(t *testing.T) {
	// ATX headings have no Lines(), falls through to child text node loop
	src := []byte("# Title\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	var line int
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			line = headingLine(h, f)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	assert.Equal(t, 1, line)
}

func TestHeadingLine_Fallback(t *testing.T) {
	// Heading with no lines and no children returns 1
	heading := ast.NewHeading(1)
	f, err := lint.NewFile("test.md", []byte("# X\n"))
	require.NoError(t, err)
	line := headingLine(heading, f)
	assert.Equal(t, 1, line)
}

func TestHeadingLine_MultiLinePosition(t *testing.T) {
	src := []byte("Some text\n\n## Heading on line 3\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	var line int
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			line = headingLine(h, f)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	assert.Equal(t, 3, line)
}

// --- extractText coverage ---

func TestExtractText_PlainText(t *testing.T) {
	src := []byte("# Hello world\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			var buf bytes.Buffer
			for c := h.FirstChild(); c != nil; c = c.NextSibling() {
				extractText(c, src, &buf)
			}
			assert.Equal(t, "Hello world", buf.String())
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

func TestExtractText_NestedEmphasis(t *testing.T) {
	src := []byte("# *emphasized* text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			var buf bytes.Buffer
			for c := h.FirstChild(); c != nil; c = c.NextSibling() {
				extractText(c, src, &buf)
			}
			assert.Equal(t, "emphasized text", buf.String())
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

func TestExtractText_NodeNoChildren(t *testing.T) {
	var buf bytes.Buffer
	node := ast.NewHeading(1)
	extractText(node, []byte("# test\n"), &buf)
	assert.Equal(t, "", buf.String())
}

func TestExtractText_Link(t *testing.T) {
	src := []byte("# [link text](url)\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			var buf bytes.Buffer
			for c := h.FirstChild(); c != nil; c = c.NextSibling() {
				extractText(c, src, &buf)
			}
			assert.Equal(t, "link text", buf.String())
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

// --- Check with setext heading having trailing punctuation ---

func TestCheck_SetextWithPunctuation(t *testing.T) {
	src := []byte("Title.\n======\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "MDS017", diags[0].RuleID)
	assert.Equal(t, 1, diags[0].Line)
}

// --- Check with empty heading ---

func TestCheck_EmptyHeading_NoDiagnostic(t *testing.T) {
	src := []byte("#\n\nSome text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	assert.Len(t, diags, 0)
}

// --- Category ---

func TestCategory(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "heading", r.Category())
}
