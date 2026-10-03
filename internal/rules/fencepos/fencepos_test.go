package fencepos

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// --- OpenLineRange: synthetic block with no fence in source ---

func TestOpenLineRange_NoFenceFound_ReturnsEndSentinel(t *testing.T) {
	// Synthetic FencedCodeBlock with Info=nil, no Lines, and no parent
	// (so PreviousSibling is nil and searchStart starts at 0). The src
	// contains no fence characters anywhere, so the scan loop exhausts
	// the source and returns the (len(src), len(src)) sentinel.
	//
	// Two src shapes exercise the two loop-exit branches: trailing
	// newline lets the for-condition fail naturally; no trailing
	// newline forces the `lineEnd >= len(src)` break.
	for _, src := range [][]byte{
		[]byte("paragraph line one\nparagraph line two\n"),
		[]byte("paragraph with no trailing newline"),
	} {
		fcb := ast.NewFencedCodeBlock(nil)
		start, end := OpenLineRange(src, fcb)
		assert.Equal(t, len(src), start, "expected sentinel start at len(src)")
		assert.Equal(t, len(src), end, "expected sentinel end at len(src)")
	}
}

// --- OpenLineRange: synthetic block, scan terminates on empty source ---

func TestOpenLineRange_EmptySource(t *testing.T) {
	// pos == len(src) means the loop body never runs. The function
	// falls through to the sentinel return.
	fcb := ast.NewFencedCodeBlock(nil)
	start, end := OpenLineRange(nil, fcb)
	assert.Equal(t, 0, start)
	assert.Equal(t, 0, end)
}

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

// TestOpenLineRange_SkipsBacktickInfoParagraph checks the scan
// fallback for a hand-built node with no parser position: a "```a`b"
// line is paragraph text (CommonMark forbids a backtick in a backtick
// fence's info string), so the scan must skip it.
func TestOpenLineRange_SkipsBacktickInfoParagraph(t *testing.T) {
	src := []byte("```a`b\n```\n")
	fcb := ast.NewFencedCodeBlock(nil)
	require.Equal(t, -1, fcb.Pos(), "a hand-built node has no position")
	start, end := OpenLineRange(src, fcb)
	assert.Equal(t, 7, start)
	assert.Equal(t, 10, end)
}

// TestOpenLineRange_PosPastEndFallsBackToScan checks that a position
// outside src is ignored in favour of the scan.
func TestOpenLineRange_PosPastEndFallsBackToScan(t *testing.T) {
	src := []byte("text\n~~~\n")
	fcb := ast.NewFencedCodeBlock(nil)
	fcb.SetPos(len(src))
	start, end := OpenLineRange(src, fcb)
	assert.Equal(t, 5, start)
	assert.Equal(t, 8, end)
}

func TestLineAround(t *testing.T) {
	src := []byte("ab\ncde\nf")
	start, end := lineAround(src, 4)
	assert.Equal(t, 3, start)
	assert.Equal(t, 6, end)
	start, end = lineAround(src, 0)
	assert.Equal(t, 0, start)
	assert.Equal(t, 2, end)
	start, end = lineAround(src, 7)
	assert.Equal(t, 7, start)
	assert.Equal(t, 8, end)
}

func TestIsFenceOpenLine(t *testing.T) {
	assert.True(t, isFenceOpenLine([]byte("```")))
	assert.True(t, isFenceOpenLine([]byte("````go")))
	assert.True(t, isFenceOpenLine([]byte("~~~a`b")), "tilde info may hold a backtick")
	assert.False(t, isFenceOpenLine([]byte("```a`b")))
	assert.False(t, isFenceOpenLine([]byte("``x")))
	assert.False(t, isFenceOpenLine([]byte("text")))
}

// emptyFenceOpenLines returns the 1-based opener line of every empty,
// info-less fenced code block in src, in document order.
func emptyFenceOpenLines(t *testing.T, src string) []int {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	var got []int
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		fcb, ok := n.(*ast.FencedCodeBlock)
		if entering && ok && fcb.Info == nil && fcb.Lines().Len() == 0 {
			got = append(got, OpenLine(f, fcb))
		}
		return ast.WalkContinue, nil
	})
	return got
}

// TestOpenLineRange_EmptyBlockPositions pins the opener line of an
// empty, info-less block in the layouts where a backward text scan
// guessed wrong: after another fenced block (it found that block's
// closer), and as the first child of a list item or block quote (it
// scanned from the start of the file).
func TestOpenLineRange_EmptyBlockPositions(t *testing.T) {
	assert.Equal(t, []int{6},
		emptyFenceOpenLines(t, "# T\n\n```go\nx\n```\n```\n```\n"),
		"after a fenced block")
	assert.Equal(t, []int{7},
		emptyFenceOpenLines(t, "# T\n\n```go\nx\n```\n\n- ```\n  ```\n"),
		"first child of a list item")
	assert.Equal(t, []int{7},
		emptyFenceOpenLines(t, "# T\n\n```go\nx\n```\n\n> ```\n> ```\n"),
		"first child of a block quote")
	assert.Equal(t, []int{6},
		emptyFenceOpenLines(t, "# T\n\n- a\n- b\n\n```\n```\n"),
		"after a list")
}

// TestOpenLineRange_InfolessContentBlockPositions pins the opener line
// of an info-less block with content whose first content segment does
// not start a line: an indented fence, a list item, a block quote. A
// walk back from that segment lands on the content line, not the
// opener, so the node position must win.
func TestOpenLineRange_InfolessContentBlockPositions(t *testing.T) {
	for _, src := range []string{
		"  ```\n  x\n  ```\n",
		"- ```\n  x\n  ```\n",
		"> ```\n> x\n> ```\n",
	} {
		f, err := lint.NewFile("test.md", []byte(src))
		require.NoError(t, err)
		var fcb *ast.FencedCodeBlock
		_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if b, ok := n.(*ast.FencedCodeBlock); ok && entering && fcb == nil {
				fcb = b
			}
			return ast.WalkContinue, nil
		})
		require.NotNil(t, fcb, src)
		require.Nil(t, fcb.Info)
		require.Equal(t, 1, fcb.Lines().Len())
		start, end := OpenLineRange(f.Source, fcb)
		assert.Equal(t, 0, start, src)
		assert.Equal(t, 5, end, src)
		assert.Equal(t, 1, OpenLine(f, fcb), src)
		assert.Equal(t, 3, CloseLine(f, fcb), src)
	}
}
