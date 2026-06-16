package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// astCodeSpanRanges parses src normally and returns the AST-derived
// code-span content and literal ranges — the byte-identical target the
// inline index must reproduce on the parse-skipped path.
func astCodeSpanRanges(t *testing.T, src string) (content, literal []Range) {
	t.Helper()
	f, err := NewFile("doc.md", []byte(src))
	require.NoError(t, err)
	return f.CodeSpanContentRanges(), f.CodeSpanLiteralRanges()
}

// indexCodeSpanRanges builds a nil-AST File from src and returns its
// inline-index-derived code-span ranges.
func indexCodeSpanRanges(src string) (content, literal []Range) {
	f := NewFileLines("doc.md", []byte(src))
	return f.CodeSpanContentRanges(), f.CodeSpanLiteralRanges()
}

// TestInlineIndex_CodeSpanEquivalence pins the inline scanner byte-identical
// to goldmark's CodeSpan node bounds across the cases the corpus exercises:
// plain spans, multi-backtick fences, the single-space trim, all-space
// spans, unclosed runs, backticks inside fenced and indented code blocks,
// adjacent spans, and escaped backticks.
func TestInlineIndex_CodeSpanEquivalence(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"plain", "before `code` after\n"},
		{"double-backtick", "a ``code`` b\n"},
		{"backtick-in-content", "a ``a`b`` c\n"},
		{"single-space-trim", "a `  x ` b\n"},
		{"both-space-trim", "a ` x ` b\n"},
		{"all-spaces", "a `   ` b\n"},
		{"empty-span", "a `` b\n"},
		{"unclosed-run", "a `code without close\n"},
		{"mismatched-len", "a ``code` more\n"},
		// One span, not two: single-backtick opener at 0; the double-backtick
		// run at positions 4-5 has length 2 ≠ 1, so it is interior content,
		// not a closer. The closer is the single backtick at 9.
		{"single-span-double-interior", "`one``two`\n"},
		{"two-spans", "`one` and `two`\n"},
		{"escaped-backtick", "a \\`not a span\\` b\n"},
		{"escaped-then-real", "a \\` then `real` b\n"},
		{"span-in-list", "- item `code` here\n"},
		{"span-after-fence", "```\nx\n```\n\nthen `code`\n"},
		{"backtick-in-fence", "```\n`not a span`\n```\n\n`real`\n"},
		{"backtick-in-indented", "    `not a span`\n\n`real`\n"},
		{"multiline-span", "a `line one\nline two` b\n"},
		{"triple-backtick-span", "a ```code``` b\n"},
		{"no-spans", "plain paragraph with no code\n"},
		{"leading-space-only", "a ` x` b\n"},
		{"trailing-space-only", "a `x ` b\n"},
		// \r is not in goldmark's isSpaceOrNewline (code_span.go), so a span
		// whose boundary byte is \r must not be trimmed by the inline scanner.
		{"crlf-boundary-no-trim", "a `\r x\r` b\n"},
		// \t is not in goldmark's isSpaceOrNewline, so a span whose sole
		// content byte is \t has a non-space boundary and is returned
		// without trimming by the early-exit path in trimCodeSpanContent.
		{"tab-only-no-trim", "a `\t` b\n"},
		// When boundaries are spaces but the interior contains \t, goldmark's
		// all-blank guard (util.IsBlank / util.IsSpace, which includes TAB)
		// fires and suppresses trim. This case exercises allCodeSpanBlank.
		{"space-tab-space-no-trim", "a ` \t ` b\n"},
		{"backtick-in-html-block", "<div>\n`not a span`\n</div>\n\n`real`\n"},
		// An opener backtick on line 1 (normal) has no valid closer because the
		// backticks on lines 2–4 are all inside a fenced code block (codeLines
		// set). scanCodeSpanAt encounters each of them and skips them via the
		// lineInSet branch (lines 156-158 of inline_index.go). The span is
		// unclosed so no code-span is recorded — matching the AST, which also
		// finds none (the fenced code interrupts the paragraph on line 1).
		{"closer-on-code-line", "`opener\n```\ncloser` here\n```\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wantContent, wantLiteral := astCodeSpanRanges(t, tc.src)
			gotContent, gotLiteral := indexCodeSpanRanges(tc.src)
			assert.Equal(t, wantContent, gotContent, "content ranges diverge from AST")
			assert.Equal(t, wantLiteral, gotLiteral, "literal ranges diverge from AST")
		})
	}
}

// TestNonInlineLines_PIAndCodeBlocks covers the merged path of nonInlineLines
// (lines 123-128 of inline_index.go): a source with both a PI block and a
// fenced code block triggers the set-merge branch (rather than the
// CodeBlockLines direct-return), exercising the CodeBlockLines copy loop and
// the PIBlockLines copy loop.
func TestNonInlineLines_PIAndCodeBlocks(t *testing.T) {
	// Line 1: inline code span (should be found by both AST and index).
	// Line 3: PI block `<?x?>` → PIBlockLines = {3} → merged path triggered.
	// Lines 5-7: fenced code block → CodeBlockLines = {5,6,7} → copy loop hit.
	src := "`real span`\n\n<?x?>\n\n```\n`code`\n```\n"
	wantContent, wantLiteral := astCodeSpanRanges(t, src)
	gotContent, gotLiteral := indexCodeSpanRanges(src)
	assert.Equal(t, wantContent, gotContent, "content ranges diverge from AST")
	assert.Equal(t, wantLiteral, gotLiteral, "literal ranges diverge from AST")
}

// TestTrimCodeSpanContent_EqualBounds covers the end<=start guard
// (lines 208-210 of inline_index.go): when start == end the function
// returns the range unchanged without inspecting the source bytes.
func TestTrimCodeSpanContent_EqualBounds(t *testing.T) {
	src := []byte("hello world")
	s, e := trimCodeSpanContent(src, 5, 5)
	assert.Equal(t, 5, s, "start must be unchanged")
	assert.Equal(t, 5, e, "end must be unchanged")
}

// TestAppendCodeSpan_ZeroWidthContent covers the ce<=cs early return
// (lines 187-189 of inline_index.go): a double-backtick opener with
// closeEnd == 2 gives contentStart=2, contentEnd=0, trimCodeSpanContent
// returns (2,0), so ce(0) <= cs(2) and nothing is appended.
func TestAppendCodeSpan_ZeroWidthContent(t *testing.T) {
	idx := &InlineIndex{}
	appendCodeSpan(idx, []byte("``"), 0, 2)
	assert.Nil(t, idx.CodeSpanContent, "zero-width content must not be recorded")
	assert.Nil(t, idx.CodeSpanLiteral, "zero-width literal must not be recorded")
}

// TestInlineIndex_NilSourceFile keeps a struct-literal File (no AST, no
// source) returning nil ranges, matching the AST guard.
func TestInlineIndex_NilSourceFile(t *testing.T) {
	f := &File{}
	assert.Nil(t, f.CodeSpanContentRanges())
	assert.Nil(t, f.CodeSpanLiteralRanges())
}

// TestInlineIndex_Memoized pins the projection to one scan per file.
func TestInlineIndex_Memoized(t *testing.T) {
	f := NewFileLines("doc.md", []byte("x `y` z\n"))
	first := InlineIndexProjection(f)
	second := InlineIndexProjection(f)
	assert.Same(t, first, second)
}
