package headingstyle

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
	src := []byte("Heading 1\n=========\n")
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

func TestHeadingLine_ATXWithEmphasis(t *testing.T) {
	// ATX heading with inline emphasis -- headingLine must walk into inline children
	src := []byte("# *emphasized title*\n")
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

func TestHeadingLine_ATXWithLink(t *testing.T) {
	// ATX heading with a link (first child is Link, not Text)
	src := []byte("# [link text](url)\n")
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

func TestHeadingLine_FallbackReturns1(t *testing.T) {
	// Construct a heading node with no children and no lines
	heading := ast.NewHeading(1)
	f, err := lint.NewFile("test.md", []byte("# X\n"))
	require.NoError(t, err)
	line := headingLine(heading, f)
	assert.Equal(t, 1, line)
}

// --- isATXHeadingNoLines ---

func TestIsATXHeadingNoLines_NoChildren(t *testing.T) {
	heading := ast.NewHeading(1)
	// No children, no lines -> defaults to true (atx)
	result := isATXHeadingNoLines(heading, []byte("# Title\n"))
	assert.True(t, result)
}

func TestIsATXHeadingNoLines_WithTextChild(t *testing.T) {
	// Parse ATX heading normally, which gives children but no Lines()
	src := []byte("# Hello\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			// ATX headings in goldmark typically have Lines().Len()==0
			if h.Lines().Len() == 0 {
				result := isATXHeadingNoLines(h, src)
				assert.True(t, result)
			}
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

// --- firstTextSegment ---

func TestFirstTextSegment_Found(t *testing.T) {
	src := []byte("# Hello world\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			seg := firstTextSegment(h)
			// Should find text within the heading
			assert.True(t, seg.Start < seg.Stop || seg.Start > 0, "expected a non-zero segment")
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

func TestFirstTextSegment_NoText(t *testing.T) {
	// A heading with no children should return zero segment
	heading := ast.NewHeading(1)
	seg := firstTextSegment(heading)
	assert.Equal(t, 0, seg.Start)
	assert.Equal(t, 0, seg.Stop)
}

// --- extractText coverage ---

func TestExtractText_PlainText(t *testing.T) {
	src := []byte("# Hello\n")
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
			assert.Equal(t, "Hello", buf.String())
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

func TestExtractText_NestedEmphasis(t *testing.T) {
	src := []byte("# *bold* text\n")
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
			assert.Equal(t, "bold text", buf.String())
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

func TestExtractText_NodeWithNoTextChildren(t *testing.T) {
	// extractText on a bare node with no children should write nothing
	var buf bytes.Buffer
	node := ast.NewHeading(1) // use as a generic container
	extractText(node, []byte("# test\n"), &buf)
	assert.Equal(t, "", buf.String())
}

// --- buildStyleReplacement coverage ---

func TestBuildStyleReplacement_ATXToSetext_Level2(t *testing.T) {
	src := []byte("## Section\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			rep, ok := buildStyleReplacement(h, src, "setext")
			assert.True(t, ok)
			assert.Contains(t, rep.newText, "Section\n-------")
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

func TestBuildStyleReplacement_ATXToSetext_Level3_NoChange(t *testing.T) {
	// Level 3+ cannot be converted to setext
	src := []byte("### Section\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			_, ok := buildStyleReplacement(h, src, "setext")
			assert.False(t, ok)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

func TestBuildStyleReplacement_SameStyle_NoChange(t *testing.T) {
	src := []byte("# Title\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			_, ok := buildStyleReplacement(h, src, "atx")
			assert.False(t, ok, "same style should not need replacement")
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

func TestBuildStyleReplacement_SetextToATX_Level1(t *testing.T) {
	src := []byte("Title\n=====\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			rep, ok := buildStyleReplacement(h, src, "atx")
			assert.True(t, ok)
			assert.Equal(t, "# Title", rep.newText)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

// --- isATXHeading coverage ---

func TestIsATXHeading_ATX(t *testing.T) {
	src := []byte("# Title\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			assert.True(t, isATXHeading(h, src))
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

func TestIsATXHeading_Setext(t *testing.T) {
	src := []byte("Title\n=====\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			assert.False(t, isATXHeading(h, src))
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

// --- Check with empty style defaults to atx ---

func TestCheck_EmptyStyle_DefaultsToATX(t *testing.T) {
	src := []byte("# Title\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: ""}
	diags := r.Check(f)
	assert.Len(t, diags, 0, "empty style should default to atx")
}

// --- Fix with empty style defaults to atx ---

func TestFix_EmptyStyle_DefaultsToATX(t *testing.T) {
	src := []byte("Heading\n=======\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: ""}
	result := r.Fix(f)
	assert.Contains(t, string(result), "# Heading")
}

// --- Fix setext to atx level 2 ---

func TestFix_SetextLevel2ToATX(t *testing.T) {
	src := []byte("Section\n-------\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "atx"}
	result := r.Fix(f)
	assert.Equal(t, "## Section", string(bytes.TrimSpace(result)))
}

// --- buildStyleReplacement: setext empty heading ---

func TestBuildStyleReplacement_ATXEmptyToSetext(t *testing.T) {
	// Empty heading: setext underline should be at least 3 chars
	src := []byte("#\n\ntext\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			rep, ok := buildStyleReplacement(h, src, "setext")
			if ok {
				assert.Contains(t, rep.newText, "===")
			}
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

// --- lineStartsWithHash edge cases ---

func TestLineStartsWithHash_AtStart(t *testing.T) {
	src := []byte("# heading\n")
	assert.True(t, lineStartsWithHash(src, 2)) // offset within # heading line
}

func TestLineStartsWithHash_NotHash(t *testing.T) {
	src := []byte("text\n")
	assert.False(t, lineStartsWithHash(src, 0))
}
