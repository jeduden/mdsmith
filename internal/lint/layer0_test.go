package lint

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// keysOf returns the sorted 1-based keys of a line set for assertions.
func keysOf(set map[int]struct{}) []int {
	out := make([]int, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

func scan(src string) *Layer0Scan {
	return Layer0(NewFileLines("doc.md", []byte(src)))
}

func TestLayer0_FencedCodeLines(t *testing.T) {
	l0 := scan("# H\n\n```go\nx := 1\n```\n")
	assert.Equal(t, []int{3, 4, 5}, keysOf(l0.CodeBlockLines))
}

func TestLayer0_TildeFencedCodeLines(t *testing.T) {
	l0 := scan("~~~\ncode\n~~~\n")
	assert.Equal(t, []int{1, 2, 3}, keysOf(l0.CodeBlockLines))
}

func TestLayer0_BacktickInInfoStringIsNotAFence(t *testing.T) {
	// A backtick inside the info string disqualifies a backtick fence, so
	// the line is ordinary prose, not code.
	l0 := scan("```go `inline`\n")
	assert.Empty(t, l0.CodeBlockLines)
}

func TestLayer0_UnclosedFenceMarksPhantomClose(t *testing.T) {
	// An unclosed fence with content marks the opening fence, its content,
	// and a phantom closing-fence line after the last content line.
	l0 := scan("```go\nx := 1\n")
	assert.Equal(t, []int{1, 2, 3}, keysOf(l0.CodeBlockLines))
}

func TestLayer0_EmptyUnclosedFenceMarksNothing(t *testing.T) {
	// An info-less, content-less fence has no source position in goldmark,
	// so the projection emits no code lines.
	l0 := scan("```\n")
	assert.Empty(t, l0.CodeBlockLines)
}

func TestLayer0_IndentedCodeAfterBlankIsCode(t *testing.T) {
	l0 := scan("para\n\n    indented code\n\nmore\n")
	assert.Equal(t, []int{3}, keysOf(l0.CodeBlockLines))
}

func TestLayer0_IndentedLineInterruptingParagraphIsNotCode(t *testing.T) {
	// Indented code cannot interrupt a paragraph (lazy continuation).
	l0 := scan("para line\n    not code\n")
	assert.Empty(t, l0.CodeBlockLines)
}

func TestLayer0_IndentedCodeTrailingBlankTrimmed(t *testing.T) {
	// Trailing blank lines are excluded from an indented code block, just
	// as goldmark trims them on close.
	l0 := scan("\n    code\n\nnext\n")
	assert.Equal(t, []int{2}, keysOf(l0.CodeBlockLines))
}

func TestLayer0_SingleLinePI(t *testing.T) {
	l0 := scan("<?toc?>\n")
	assert.Equal(t, []int{1}, keysOf(l0.PIBlockLines))
}

func TestLayer0_MultiLinePI(t *testing.T) {
	l0 := scan("<?catalog\nglob: docs\n?>\n")
	assert.Equal(t, []int{1, 2, 3}, keysOf(l0.PIBlockLines))
}

func TestLayer0_ClosingDirectivePIInterruptsParagraph(t *testing.T) {
	// A paired directive: the body line is prose, the <?/include?> marker
	// is its own single-line PI interrupting the paragraph.
	l0 := scan("<?include\nf: x\n?>\nbody\n<?/include?>\n")
	assert.Equal(t, []int{1, 2, 3, 5}, keysOf(l0.PIBlockLines))
}

func TestLayer0_NamelessPIIsNotPI(t *testing.T) {
	// "<? ?>" has no name; the PI parser rejects it.
	l0 := scan("<? ?>\n")
	assert.Empty(t, l0.PIBlockLines)
}

func TestLayer0_HTMLCommentSuppressesIndentedCode(t *testing.T) {
	// Indented lines inside an HTML comment are not indented code.
	l0 := scan("<!-- comment\n    indented inside comment\n-->\n")
	assert.Empty(t, l0.CodeBlockLines)
}

func TestLayer0_LeadingDelimiterPairIsNotFrontMatter(t *testing.T) {
	// The scan never strips front matter (the engine strips it before
	// building the File). A body that opens with a `---` thematic break and
	// contains a later `---` must still surface the fenced code between
	// them, matching goldmark.
	l0 := scan("---\n```\ncode\n```\n---\n")
	assert.Equal(t, []int{2, 3, 4}, keysOf(l0.CodeBlockLines))
}

func TestLayer0_BlockquotedFencedCodeIsCode(t *testing.T) {
	l0 := scan("> ```\n> code\n> ```\n")
	assert.Equal(t, []int{1, 2, 3}, keysOf(l0.CodeBlockLines))
}

func TestLayer0_NestedBlockquotedFencedCodeIsCode(t *testing.T) {
	// A fence two quote levels deep must still be found — the code-capable
	// guard descends through nested `>` markers.
	l0 := scan("> > ```\n> > code\n> > ```\n")
	assert.Equal(t, []int{1, 2, 3}, keysOf(l0.CodeBlockLines))
}

func TestLayer0_IndentedLazyContinuationAfterQuoteIsNotCode(t *testing.T) {
	// An indented line that lazily continues a block quote paragraph is not
	// an indented code block (goldmark: lazy continuation, not code).
	l0 := scan("> para line\n    lazy continuation\n")
	assert.Empty(t, l0.CodeBlockLines)
}

func TestLayer0_IndentedCodeKeepsInteriorBlank(t *testing.T) {
	// A blank line interior to an indented code block stays code (goldmark
	// trims only trailing blanks).
	l0 := scan("x\n\n    a\n   \n    b\nend\n")
	assert.Equal(t, []int{3, 4, 5}, keysOf(l0.CodeBlockLines))
}

func TestLayer0_BlockSpansClassifyHeadingsAndQuotes(t *testing.T) {
	l0 := scan("# Heading\n\n> a quote\n\n- item\n")
	kinds := map[BlockKind]int{}
	for _, sp := range l0.BlockSpans {
		kinds[sp.Kind]++
	}
	assert.Equal(t, 1, kinds[BlockATXHeading])
	assert.Equal(t, 1, kinds[BlockQuote])
	assert.Equal(t, 1, kinds[BlockList])
}

func TestLayer0_SetextHeadingSpan(t *testing.T) {
	l0 := scan("Title\n=====\n\nbody\n")
	var found bool
	for _, sp := range l0.BlockSpans {
		if sp.Kind == BlockSetextHeading {
			found = true
			assert.Equal(t, 1, sp.Start)
			assert.Equal(t, 2, sp.End)
		}
	}
	assert.True(t, found, "expected a setext heading span")
}

func TestLayer0_ThematicBreakSpan(t *testing.T) {
	l0 := scan("a\n\n---\n\nb\n")
	var found bool
	for _, sp := range l0.BlockSpans {
		if sp.Kind == BlockThematicBreak {
			found = true
		}
	}
	assert.True(t, found, "expected a thematic break span")
}

func TestLayer0_CachedAcrossCalls(t *testing.T) {
	f := NewFileLines("doc.md", []byte("```\ncode\n```\n"))
	a := Layer0(f)
	b := Layer0(f)
	require.Same(t, a, b, "Layer0 must memoize the same scan")
}

func TestLayer0_NestedBlockquoteDepth(t *testing.T) {
	l0 := scan("> > nested\n")
	require.Len(t, l0.BlockSpans, 1)
	assert.Equal(t, BlockQuote, l0.BlockSpans[0].Kind)
	assert.Equal(t, 2, l0.BlockSpans[0].Depth)
}
