package blanklinearoundheadings

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
)

// --- isNonBlankLine coverage ---

func TestIsNonBlankLine_NonBlank(t *testing.T) {
	lines := [][]byte{[]byte("hello")}
	assert.True(t, isNonBlankLine(lines, 0))
}

func TestIsNonBlankLine_Blank(t *testing.T) {
	lines := [][]byte{[]byte(""), []byte("   "), []byte("  \t  ")}
	assert.False(t, isNonBlankLine(lines, 0))
	assert.False(t, isNonBlankLine(lines, 1))
	assert.False(t, isNonBlankLine(lines, 2))
}

func TestIsNonBlankLine_OutOfBoundsNegative(t *testing.T) {
	lines := [][]byte{[]byte("hello")}
	assert.False(t, isNonBlankLine(lines, -1))
}

func TestIsNonBlankLine_OutOfBoundsPositive(t *testing.T) {
	lines := [][]byte{[]byte("hello")}
	assert.False(t, isNonBlankLine(lines, 1))
}

func TestIsNonBlankLine_EmptySlice(t *testing.T) {
	assert.False(t, isNonBlankLine(nil, 0))
}

// --- headingLastLine coverage ---

func TestHeadingLastLine_ATXHeading(t *testing.T) {
	src := []byte("# Title\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			last := headingLastLine(h, f)
			assert.Equal(t, 1, last)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

func TestHeadingLastLine_SetextHeading(t *testing.T) {
	src := []byte("Title\n=====\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			last := headingLastLine(h, f)
			assert.Equal(t, 2, last, "setext heading should span 2 lines")
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

func TestHeadingLastLine_SetextLevel2(t *testing.T) {
	src := []byte("Section\n-------\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			last := headingLastLine(h, f)
			assert.Equal(t, 2, last)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

// --- isSetextHeading coverage ---

func TestIsSetextHeading_Setext(t *testing.T) {
	src := []byte("Title\n=====\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			assert.True(t, isSetextHeading(h, src))
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

func TestIsSetextHeading_ATX(t *testing.T) {
	src := []byte("# Title\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			// ATX headings with Lines().Len()==0 return false
			assert.False(t, isSetextHeading(h, src))
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

func TestIsSetextHeading_NoLines(t *testing.T) {
	heading := ast.NewHeading(1)
	assert.False(t, isSetextHeading(heading, []byte("# X\n")))
}

// --- Check with setext heading ---

func TestCheck_Heading_BlankLineAfterRequired(t *testing.T) {
	// Test the "after" case: ATX heading parsed but no blank line after.
	src := []byte("# ATX heading\nmore text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	foundAfter := false
	for _, d := range diags {
		if strings.Contains(d.Message, "blank line after") {
			foundAfter = true
		}
	}
	assert.True(t, foundAfter, "expected blank-after diagnostic")
}

// --- Fix with heading needing blank lines ---

func TestFix_Heading_InsertsBlankLines(t *testing.T) {
	src := []byte("# Title\nmore text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	result := r.Fix(f)
	assert.Contains(t, string(result), "\n\nmore text")
}

// --- headingLastLine fallback ---

func TestHeadingLastLine_NoLines_FallsBackToAstutil(t *testing.T) {
	// Synthetic heading with no Lines() set exercises the fallback branch
	// in headingLastLine that delegates to astutil.HeadingLine.
	heading := ast.NewHeading(1)
	f, err := lint.NewFile("test.md", []byte("# X\n"))
	require.NoError(t, err)
	last := headingLastLine(heading, f)
	assert.Equal(t, 1, last)
}

// --- Category ---

func TestCategory(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "heading", r.Category())
}
