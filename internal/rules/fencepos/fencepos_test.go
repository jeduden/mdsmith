package fencepos

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/goldmark/text"
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

// --- OpenLineRange: synthetic block with no fence in source ---

func TestOpenLineRange_NoFenceFound_ReturnsEndSentinel(t *testing.T) {
	// Synthetic FencedCodeBlock with no position, Info=nil, and no
	// Lines. The src holds no fence line, so the scan exhausts the source
	// and returns the (len(src), len(src)) sentinel, with and without a
	// trailing newline.
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

// TestOpenLine_EmptyInfolessFence pins the opening line of an empty
// fence with no info string, which goldmark gives no info or content
// segment, in layouts where scanning forward from the previous sibling
// would land on an earlier fence-looking line: after a list (no Lines of
// its own), after a fenced block (whose Lines stop before its closer),
// inside a block quote, and in a list item after a tab.
func TestOpenLine_EmptyInfolessFence(t *testing.T) {
	tests := []struct {
		src  string
		want []int
	}{
		{"```js\nx\n```\n\n- item\n\n```\n```\n", []int{1, 7}},
		{"```\nx\n```\n```\n```\n", []int{1, 4}},
		{"> ```\n> ```\n", []int{1}},
		{"para\n\n~~~\n~~~\n", []int{3}},
	}
	for _, tt := range tests {
		f, err := lint.NewFile("test.md", []byte(tt.src))
		require.NoError(t, err)
		var got []int
		_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if fcb, ok := n.(*ast.FencedCodeBlock); ok && entering {
				got = append(got, OpenLine(f, fcb))
			}
			return ast.WalkContinue, nil
		})
		assert.Equal(t, tt.want, got, "%q", tt.src)
	}
}

func TestOpenLineRange_SyntheticBlockFindsFirstFence(t *testing.T) {
	src := []byte("text\n  ~~~\nx\n")
	start, end := OpenLineRange(src, ast.NewFencedCodeBlock(nil))
	assert.Equal(t, "  ~~~", string(src[start:end]))
}

func TestLineAround(t *testing.T) {
	src := []byte("ab\ncd\nef")
	for _, tt := range []struct{ p, start, end int }{
		{0, 0, 2}, {1, 0, 2}, {3, 3, 5}, {4, 3, 5}, {7, 6, 8}, {8, 6, 8},
	} {
		start, end := lineAround(src, tt.p)
		assert.Equal(t, [2]int{tt.start, tt.end}, [2]int{start, end}, "p=%d", tt.p)
	}
}

func TestLineLen(t *testing.T) {
	assert.Equal(t, 2, lineLen([]byte("ab\ncd")))
	assert.Equal(t, 2, lineLen([]byte("ab")))
	assert.Equal(t, 0, lineLen([]byte("\n")))
	assert.Equal(t, 0, lineLen(nil))
}

// TestOpenLineRange_SyntheticBlockWithInfo pins the hand-built path for
// a block with no position but an info segment: the opening line is
// the line holding the info string.
func TestOpenLineRange_SyntheticBlockWithInfo(t *testing.T) {
	src := []byte("text\n```go\nx\n```\n")
	info := ast.NewText()
	info.Segment = text.NewSegment(8, 10)
	start, end := OpenLineRange(src, ast.NewFencedCodeBlock(info))
	assert.Equal(t, "```go", string(src[start:end]))
}

// TestOpenLineRange_SyntheticBlockWithLines pins the hand-built path for
// a block with no position or info but content lines: the opening line
// ends just before the first content line, and a first content line at
// offset 0 (no newline before it) is its own line.
func TestOpenLineRange_SyntheticBlockWithLines(t *testing.T) {
	src := []byte("~~~\nbody\n~~~\n")
	fcb := ast.NewFencedCodeBlock(nil)
	segs := text.NewSegments()
	segs.Append(text.NewSegment(4, 9))
	fcb.SetLines(segs)
	start, end := OpenLineRange(src, fcb)
	assert.Equal(t, "~~~", string(src[start:end]))

	fcb = ast.NewFencedCodeBlock(nil)
	segs = text.NewSegments()
	segs.Append(text.NewSegment(0, 5))
	fcb.SetLines(segs)
	start, end = OpenLineRange([]byte("body\n"), fcb)
	assert.Equal(t, 0, start)
	assert.Equal(t, 4, end)
}

// TestCloseLineRange_NoTrailingNewline covers a closing fence that ends
// the file with no newline: the range must stop at the end of the
// source.
func TestCloseLineRange_NoTrailingNewline(t *testing.T) {
	for src, want := range map[string]string{
		"```go\ncode\n```": "```",
		"~~~\ncode\n~~~~~": "~~~~~",
	} {
		f, err := lint.NewFile("test.md", []byte(src))
		require.NoError(t, err)
		visited := false
		_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if fcb, ok := n.(*ast.FencedCodeBlock); ok && entering {
				visited = true
				_, openEnd := OpenLineRange(f.Source, fcb)
				s, e := CloseLineRange(f.Source, fcb, openEnd)
				assert.Equal(t, want, string(f.Source[s:e]), src)
				return ast.WalkStop, nil
			}
			return ast.WalkContinue, nil
		})
		assert.True(t, visited, "no fenced code block parsed from %q", src)
	}
}

// TestCloseLineRange_SegmentPastSource covers a hand-built block whose
// content segment ends beyond the source: the range clamps to an empty
// range at that offset instead of slicing out of bounds.
func TestCloseLineRange_SegmentPastSource(t *testing.T) {
	fcb := ast.NewFencedCodeBlock(nil)
	fcb.Lines().Append(text.NewSegment(5, 40))
	s, e := CloseLineRange([]byte("```\n"), fcb, 3)
	assert.Equal(t, 40, s)
	assert.Equal(t, 40, e)
}
