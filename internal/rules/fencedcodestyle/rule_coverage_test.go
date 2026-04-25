package fencedcodestyle

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
)

// --- FenceCharAt coverage ---

func TestFenceCharAt_Backtick(t *testing.T) {
	src := []byte("```go\n")
	assert.Equal(t, byte('`'), FenceCharAt(src, 0))
}

func TestFenceCharAt_Tilde(t *testing.T) {
	src := []byte("~~~go\n")
	assert.Equal(t, byte('~'), FenceCharAt(src, 0))
}

func TestFenceCharAt_LeadingSpaces(t *testing.T) {
	src := []byte("   ```go\n")
	assert.Equal(t, byte('`'), FenceCharAt(src, 0))
}

func TestFenceCharAt_NotFenceChar(t *testing.T) {
	src := []byte("not a fence\n")
	assert.Equal(t, byte(0), FenceCharAt(src, 0))
}

func TestFenceCharAt_PastEnd(t *testing.T) {
	src := []byte("   ")
	assert.Equal(t, byte(0), FenceCharAt(src, 0))
}

func TestFenceCharAt_EmptySource(t *testing.T) {
	src := []byte("")
	assert.Equal(t, byte(0), FenceCharAt(src, 0))
}

// --- FenceOpenLine coverage ---

func TestFenceOpenLine(t *testing.T) {
	src := []byte("# Title\n\n```go\ncode\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if fcb, ok := n.(*ast.FencedCodeBlock); ok {
			line := FenceOpenLine(f, fcb)
			assert.Equal(t, 3, line)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

// --- FenceCloseLine coverage ---

func TestFenceCloseLine(t *testing.T) {
	src := []byte("```go\ncode\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if fcb, ok := n.(*ast.FencedCodeBlock); ok {
			line := FenceCloseLine(f, fcb)
			assert.Equal(t, 3, line)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

// --- FenceOpenLineRange coverage ---

func TestFenceOpenLineRange_WithInfo(t *testing.T) {
	src := []byte("```go\ncode\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if fcb, ok := n.(*ast.FencedCodeBlock); ok {
			start, end := FenceOpenLineRange(f.Source, fcb)
			assert.Equal(t, 0, start)
			assert.Equal(t, 5, end) // "```go" is 5 bytes
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

func TestFenceOpenLineRange_NoInfo(t *testing.T) {
	src := []byte("```\ncode\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if fcb, ok := n.(*ast.FencedCodeBlock); ok {
			start, end := FenceOpenLineRange(f.Source, fcb)
			assert.Equal(t, 0, start)
			// "```" is 3 bytes
			assert.Equal(t, 3, end)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

// --- FenceCloseLineRange coverage ---

func TestFenceCloseLineRange_WithContent(t *testing.T) {
	src := []byte("```go\ncode\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if fcb, ok := n.(*ast.FencedCodeBlock); ok {
			_, openEnd := FenceOpenLineRange(f.Source, fcb)
			closeStart, closeEnd := FenceCloseLineRange(f.Source, fcb, openEnd)
			closeLine := string(f.Source[closeStart:closeEnd])
			assert.Equal(t, "```", closeLine)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

func TestFenceCloseLineRange_EmptyBlock(t *testing.T) {
	src := []byte("```\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if fcb, ok := n.(*ast.FencedCodeBlock); ok {
			_, openEnd := FenceOpenLineRange(f.Source, fcb)
			closeStart, closeEnd := FenceCloseLineRange(f.Source, fcb, openEnd)
			assert.True(t, closeStart <= closeEnd)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

// --- FenceLines coverage ---

func TestFenceLines(t *testing.T) {
	src := []byte("```go\nline1\nline2\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if fcb, ok := n.(*ast.FencedCodeBlock); ok {
			openStart, openEnd, closeStart, closeEnd := FenceLines(f.Source, fcb)
			assert.Equal(t, 0, openStart)
			assert.True(t, openEnd > openStart, "openEnd should be after openStart")
			assert.True(t, closeStart >= openEnd, "closeStart should be at or after openEnd")
			assert.True(t, closeEnd >= closeStart, "closeEnd should be at or after closeStart")
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

// --- Category ---

func TestCategory(t *testing.T) {
	r := &Rule{Style: "backtick"}
	assert.Equal(t, "code", r.Category())
}

// --- Multiple blocks ---

func TestFenceOpenLine_SecondBlock(t *testing.T) {
	src := []byte("```\nfirst\n```\n\n```\nsecond\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	var lines []int
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if fcb, ok := n.(*ast.FencedCodeBlock); ok {
			lines = append(lines, FenceOpenLine(f, fcb))
		}
		return ast.WalkContinue, nil
	})
	require.Len(t, lines, 2)
	assert.Equal(t, 1, lines[0])
	assert.Equal(t, 5, lines[1])
}

// --- replaceFenceChars with leading spaces ---

func TestReplaceFenceChars_LeadingSpaces(t *testing.T) {
	// A fence line with leading spaces: "  ~~~go" -> "  ```go"
	line := []byte("  ~~~go")
	result := replaceFenceChars(line, '`')
	assert.Equal(t, []byte("  ```go"), result)
}

// --- fenceOpenLineRange: previous sibling branch ---

func TestFenceOpenLineRange_EmptyBlockWithPreviousSibling(t *testing.T) {
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
		start, end := FenceOpenLineRange(f.Source, fcb)
		assert.True(t, start < len(f.Source), "expected start within source")
		assert.True(t, end >= start, "expected end >= start")
		found = true
		return ast.WalkStop, nil
	})
	assert.True(t, found, "expected to find a fenced code block")
}

// --- fenceOpenLineRange: info with trailing non-newline chars ---

func TestFenceOpenLineRange_InfoWithTrailingSpace(t *testing.T) {
	// Fence with trailing spaces after info string: ```go   \n
	// The lineEnd loop at line 178 advances past the trailing spaces.
	src := []byte("```go   \ncode\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if fcb, ok := n.(*ast.FencedCodeBlock); ok {
			start, end := FenceOpenLineRange(f.Source, fcb)
			// Opening line is "```go   " — end should be past the trailing spaces
			assert.Equal(t, 0, start)
			assert.True(t, end >= 5, "end should be past the info string")
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}

// --- Fix with empty block after paragraph (exercises previousSibling path) ---

func TestFix_EmptyTildeBlockAfterParagraph(t *testing.T) {
	src := []byte("paragraph\n\n~~~\n~~~\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "backtick"}
	result := r.Fix(f)
	assert.Equal(t, "paragraph\n\n```\n```\n", string(result))
}

// --- Tilde fence lines ---

func TestFenceLines_Tilde(t *testing.T) {
	src := []byte("~~~\ncode\n~~~\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if fcb, ok := n.(*ast.FencedCodeBlock); ok {
			openStart, openEnd, closeStart, closeEnd := FenceLines(f.Source, fcb)
			openLine := string(f.Source[openStart:openEnd])
			closeLine := string(f.Source[closeStart:closeEnd])
			assert.Equal(t, "~~~", openLine)
			assert.Equal(t, "~~~", closeLine)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
}
