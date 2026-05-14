package fencepos

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
)

// --- CharAt coverage ---

func TestCharAt_Backtick(t *testing.T) {
	src := []byte("```go\n")
	assert.Equal(t, byte('`'), CharAt(src, 0))
}

func TestCharAt_Tilde(t *testing.T) {
	src := []byte("~~~go\n")
	assert.Equal(t, byte('~'), CharAt(src, 0))
}

func TestCharAt_LeadingSpaces(t *testing.T) {
	src := []byte("   ```go\n")
	assert.Equal(t, byte('`'), CharAt(src, 0))
}

func TestCharAt_NotFenceChar(t *testing.T) {
	src := []byte("not a fence\n")
	assert.Equal(t, byte(0), CharAt(src, 0))
}

func TestCharAt_PastEnd(t *testing.T) {
	src := []byte("   ")
	assert.Equal(t, byte(0), CharAt(src, 0))
}

func TestCharAt_EmptySource(t *testing.T) {
	src := []byte("")
	assert.Equal(t, byte(0), CharAt(src, 0))
}

// --- OpenLine coverage ---

func TestOpenLine(t *testing.T) {
	src := []byte("# Title\n\n```go\ncode\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if fcb, ok := n.(*ast.FencedCodeBlock); ok {
			line := OpenLine(f, fcb)
			assert.Equal(t, 3, line)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

func TestOpenLine_SecondBlock(t *testing.T) {
	src := []byte("```\nfirst\n```\n\n```\nsecond\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	var lines []int
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if fcb, ok := n.(*ast.FencedCodeBlock); ok {
			lines = append(lines, OpenLine(f, fcb))
		}
		return ast.WalkContinue, nil
	})
	require.Len(t, lines, 2)
	assert.Equal(t, 1, lines[0])
	assert.Equal(t, 5, lines[1])
}

// --- CloseLine coverage ---

func TestCloseLine(t *testing.T) {
	src := []byte("```go\ncode\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if fcb, ok := n.(*ast.FencedCodeBlock); ok {
			line := CloseLine(f, fcb)
			assert.Equal(t, 3, line)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

// --- OpenLineRange coverage ---

func TestOpenLineRange_WithInfo(t *testing.T) {
	src := []byte("```go\ncode\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if fcb, ok := n.(*ast.FencedCodeBlock); ok {
			start, end := OpenLineRange(f.Source, fcb)
			assert.Equal(t, 0, start)
			assert.Equal(t, 5, end) // "```go" is 5 bytes
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

func TestOpenLineRange_NoInfo(t *testing.T) {
	src := []byte("```\ncode\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if fcb, ok := n.(*ast.FencedCodeBlock); ok {
			start, end := OpenLineRange(f.Source, fcb)
			assert.Equal(t, 0, start)
			// "```" is 3 bytes
			assert.Equal(t, 3, end)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

// --- CloseLineRange coverage ---

func TestCloseLineRange_WithContent(t *testing.T) {
	src := []byte("```go\ncode\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if fcb, ok := n.(*ast.FencedCodeBlock); ok {
			_, openEnd := OpenLineRange(f.Source, fcb)
			closeStart, closeEnd := CloseLineRange(f.Source, fcb, openEnd)
			closeLine := string(f.Source[closeStart:closeEnd])
			assert.Equal(t, "```", closeLine)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

func TestCloseLineRange_EmptyBlock(t *testing.T) {
	src := []byte("```\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if fcb, ok := n.(*ast.FencedCodeBlock); ok {
			_, openEnd := OpenLineRange(f.Source, fcb)
			closeStart, closeEnd := CloseLineRange(f.Source, fcb, openEnd)
			assert.True(t, closeStart <= closeEnd)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

// --- Combined OpenLineRange + CloseLineRange ---

func TestRanges_FullBlock(t *testing.T) {
	src := []byte("```go\nline1\nline2\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if fcb, ok := n.(*ast.FencedCodeBlock); ok {
			openStart, openEnd := OpenLineRange(f.Source, fcb)
			closeStart, closeEnd := CloseLineRange(f.Source, fcb, openEnd)
			assert.Equal(t, 0, openStart)
			assert.True(t, openEnd > openStart, "openEnd should be after openStart")
			assert.True(t, closeStart >= openEnd, "closeStart should be at or after openEnd")
			assert.True(t, closeEnd >= closeStart, "closeEnd should be at or after closeStart")
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

func TestRanges_TildeFence(t *testing.T) {
	src := []byte("~~~\ncode\n~~~\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if fcb, ok := n.(*ast.FencedCodeBlock); ok {
			openStart, openEnd := OpenLineRange(f.Source, fcb)
			closeStart, closeEnd := CloseLineRange(f.Source, fcb, openEnd)
			openLine := string(f.Source[openStart:openEnd])
			closeLine := string(f.Source[closeStart:closeEnd])
			assert.Equal(t, "~~~", openLine)
			assert.Equal(t, "~~~", closeLine)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

// --- lastByteOfNodeStop coverage ---

func TestLastByteOfNodeStop_Paragraph(t *testing.T) {
	src := []byte("paragraph text\n\n```\ncode\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	// Find the paragraph node
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if _, ok := n.(*ast.Paragraph); ok {
			stop := lastByteOfNodeStop(f.Source, n)
			assert.Greater(t, stop, 0, "paragraph should have non-zero stop")
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

func TestLastByteOfNodeStop_NoLines(t *testing.T) {
	// A node with no Lines() should return 0
	heading := ast.NewHeading(1)
	result := lastByteOfNodeStop([]byte("# test\n"), heading)
	assert.Equal(t, 0, result)
}

// --- OpenLineRange: previous sibling branch ---

func TestOpenLineRange_EmptyBlockWithPreviousSibling(t *testing.T) {
	// Empty tilde code block after a paragraph: PreviousSibling() is non-nil.
	src := []byte("paragraph\n\n~~~\n~~~\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	found := false
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		fcb, ok := n.(*ast.FencedCodeBlock)
		if !ok {
			return ast.WalkContinue, nil
		}
		start, end := OpenLineRange(f.Source, fcb)
		assert.True(t, start < len(f.Source), "expected start within source")
		assert.True(t, end >= start, "expected end >= start")
		found = true
		return ast.WalkStop, nil
	})
	assert.True(t, found, "expected to find a fenced code block")
}

// --- OpenLineRange: info with trailing non-newline chars ---

func TestOpenLineRange_InfoWithTrailingSpace(t *testing.T) {
	// Fence with trailing spaces after info string: ```go   \n
	// The lineEnd loop advances past the trailing spaces.
	src := []byte("```go   \ncode\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if fcb, ok := n.(*ast.FencedCodeBlock); ok {
			start, end := OpenLineRange(f.Source, fcb)
			// Opening line is "```go   " — end should be past the trailing spaces
			assert.Equal(t, 0, start)
			assert.True(t, end >= 5, "end should be past the info string")
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}
