package lint

import (
	"testing"

	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCodeSpanFile(t *testing.T, src string) *File {
	t.Helper()
	f, err := NewFile("doc.md", []byte(src))
	require.NoError(t, err)
	return f
}

func TestCodeSpanContentRanges_Basic(t *testing.T) {
	src := "before `code` after\n"
	f := newCodeSpanFile(t, src)
	ranges := f.CodeSpanContentRanges()
	require.Len(t, ranges, 1)
	assert.Equal(t, "code", src[ranges[0].Start:ranges[0].End])
}

func TestCodeSpanLiteralRanges_IncludesBackticks(t *testing.T) {
	src := "before ``code`` after\n"
	f := newCodeSpanFile(t, src)
	ranges := f.CodeSpanLiteralRanges()
	require.Len(t, ranges, 1)
	assert.Equal(t, "``code``", src[ranges[0].Start:ranges[0].End])
}

func TestCodeSpanRanges_NoSpans(t *testing.T) {
	f := newCodeSpanFile(t, "plain paragraph\n")
	assert.Empty(t, f.CodeSpanContentRanges())
	assert.Empty(t, f.CodeSpanLiteralRanges())
}

func TestCodeSpanRanges_MultipleAndNested(t *testing.T) {
	src := "a `one` b `two` c\n\n- item `three`\n"
	f := newCodeSpanFile(t, src)
	content := f.CodeSpanContentRanges()
	require.Len(t, content, 3)
	assert.Equal(t, "one", src[content[0].Start:content[0].End])
	assert.Equal(t, "two", src[content[1].Start:content[1].End])
	assert.Equal(t, "three", src[content[2].Start:content[2].End])
	literal := f.CodeSpanLiteralRanges()
	require.Len(t, literal, 3)
	assert.Equal(t, "`one`", src[literal[0].Start:literal[0].End])
}

func TestCodeSpanRanges_MemoizedSameSlice(t *testing.T) {
	f := newCodeSpanFile(t, "x `y` z\n")
	first := f.CodeSpanContentRanges()
	second := f.CodeSpanContentRanges()
	require.Len(t, first, 1)
	// Same backing array: the walk must run once per file.
	assert.Same(t, &first[0], &second[0])
	lit1 := f.CodeSpanLiteralRanges()
	lit2 := f.CodeSpanLiteralRanges()
	require.Len(t, lit1, 1)
	assert.Same(t, &lit1[0], &lit2[0])
}

func TestCodeSpanRanges_NilAST(t *testing.T) {
	f := &File{}
	assert.Nil(t, f.CodeSpanContentRanges())
	assert.Nil(t, f.CodeSpanLiteralRanges())
}

// TestCodeSpanRanges_NilASTEquivalence pins the parse-skipped projection
// (served from the shared run-grouped inline parse, InlineBlocks) byte-
// identical to the AST walk across the shapes the corpus exercises — in
// particular the block-boundary cases a flat byte scan got wrong: a code
// span cannot cross a blank line, an interrupting heading or fence, a
// thematic break, a nested container lead, or a setext underline, but it may
// span the continuation lines of its own paragraph, block quote, or list
// item.
func TestCodeSpanRanges_NilASTEquivalence(t *testing.T) {
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
		{"single-span-double-interior", "`one``two`\n"},
		{"two-spans", "`one` and `two`\n"},
		{"escaped-backtick", "a \\`not a span\\` b\n"},
		{"span-in-list", "- item `code` here\n"},
		{"span-after-fence", "```\nx\n```\n\nthen `code`\n"},
		{"backtick-in-fence", "```\n`not a span`\n```\n\n`real`\n"},
		{"backtick-in-indented", "    `not a span`\n\n`real`\n"},
		{"multiline-span", "a `line one\nline two` b\n"},
		{"triple-backtick-span", "a ```code``` b\n"},
		{"backtick-in-html-block", "<div>\n`not a span`\n</div>\n\n`real`\n"},
		{"crlf-boundary-no-trim", "a `\r x\r` b\n"},
		{"tab-only-no-trim", "a `\t` b\n"},
		{"space-tab-space-no-trim", "a ` \t ` b\n"},
		// Block boundaries: no span may cross these.
		{"open-blank-close", "a `b\n\nc` d\n"},
		{"open-blank-close-leading", "`a\n\nb`\n"},
		{"phantom-then-real", "`a\n\nb` and `c` real\n"},
		{"open-heading-close", "`open\n# heading\nclose`\n"},
		{"open-fence-close", "`open\n```\ncode\n```\nclose`\n"},
		{"open-thematic-close", "`a\n***\nb`\n"},
		{"open-quote-close", "`a\n> quote\nb`\n"},
		{"open-list-close", "`a\n- list\nb`\n"},
		{"open-setext-close", "`a\n---\nb`\n"},
		{"open-setext-equals", "`a\nsetext\n===\nb`\n"},
		// Continuation lines of the opener's own block: span may extend.
		{"multiline-three", "`a\nb\nc`\n"},
		{"quote-multiline", "> `a\n> b\n> c`\n"},
		{"list-marker-line-span", "- `a\n  b`\n"},
		{"ordered-list-span", "1. `a\n   b`\n"},
		{"list-continuation-span", "- item one\n  `a\n  b`\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			astFile, err := NewFile("doc.md", []byte(tc.src))
			require.NoError(t, err)
			lines := NewFileLines("doc.md", []byte(tc.src))
			assert.Equal(t, astFile.CodeSpanContentRanges(), lines.CodeSpanContentRanges(),
				"content ranges diverge from AST")
			assert.Equal(t, astFile.CodeSpanLiteralRanges(), lines.CodeSpanLiteralRanges(),
				"literal ranges diverge from AST")
		})
	}
}

// TestCollectCodeSpanRangesInto_NilNode pins the nil-node guard on
// the recursive helper: a struct-literal *File with no AST must stay
// safe.
func TestCollectCodeSpanRangesInto_NilNode(t *testing.T) {
	var content, literal []Range
	collectCodeSpanRangesInto(nil, nil, &content, &literal)
	assert.Empty(t, content)
	assert.Empty(t, literal)
}

// TestCodeSpanTextBounds_NonTextChild pins the inline-code-span case
// where a child is not an *ast.Text (e.g. a synthetic emphasis nested
// inside the span). The helper skips non-Text children; goldmark's own
// parsed code spans contain only *ast.Text children, which is why this
// branch stays cold without a synthetic child.
func TestCodeSpanTextBounds_NonTextChild(t *testing.T) {
	f := newCodeSpanFile(t, "`code`\n")
	var span *ast.CodeSpan
	findCodeSpanForTest(f.AST, &span)
	require.NotNil(t, span, "fixture must produce a code span")
	span.AppendChild(span, ast.NewEmphasis(1))
	first, last := codeSpanTextBounds(span)
	assert.GreaterOrEqual(t, first, 0)
	assert.GreaterOrEqual(t, last, first)
}

// findCodeSpanForTest returns the first *ast.CodeSpan under n.
func findCodeSpanForTest(n ast.Node, out **ast.CodeSpan) {
	if *out != nil {
		return
	}
	if cs, ok := n.(*ast.CodeSpan); ok {
		*out = cs
		return
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		findCodeSpanForTest(c, out)
		if *out != nil {
			return
		}
	}
}

func TestLineStartOffset(t *testing.T) {
	src := "ab\ncdef\n\nx\n"
	f := newCodeSpanFile(t, src)
	assert.Equal(t, 0, f.LineStartOffset(0)) // "ab"
	assert.Equal(t, 3, f.LineStartOffset(1)) // "cdef"
	assert.Equal(t, 8, f.LineStartOffset(2)) // ""
	assert.Equal(t, 9, f.LineStartOffset(3)) // "x"
	// Past the last line: clamped to len(Source).
	assert.Equal(t, len(src), f.LineStartOffset(99))
	assert.Equal(t, 0, f.LineStartOffset(-1), "negative index clamps to 0")
}

func TestLineStrings_ZeroCopyViews(t *testing.T) {
	src := "alpha\nbeta\n\ngamma\n"
	f := newCodeSpanFile(t, src)
	ls := f.LineStrings()
	require.Len(t, ls, len(f.Lines))
	assert.Equal(t, "alpha", ls[0])
	assert.Equal(t, "beta", ls[1])
	assert.Equal(t, "", ls[2])
	assert.Equal(t, "gamma", ls[3])
	assert.Equal(t, "", ls[4], "trailing empty split element preserved")
	// Memoized: same backing on the second call.
	again := f.LineStrings()
	assert.Same(t, &ls[0], &again[0])
	// Zero further allocations once built.
	assert.Zero(t, testing.AllocsPerRun(50, func() { _ = f.LineStrings() }))
}

func TestLineStrings_EmptyFile(t *testing.T) {
	f := &File{}
	assert.Nil(t, f.LineStrings())
}

// TestBytesView pins the zero-copy string view: content is preserved,
// empty/nil inputs return empty string, and the conversion allocates nothing.
func TestBytesView(t *testing.T) {
	t.Run("nonEmpty", func(t *testing.T) {
		b := []byte("hello")
		s := BytesView(b)
		assert.Equal(t, "hello", s)
		assert.Equal(t, len(b), len(s))
	})
	t.Run("nilBytes", func(t *testing.T) {
		assert.Equal(t, "", BytesView(nil))
	})
	t.Run("emptySlice", func(t *testing.T) {
		assert.Equal(t, "", BytesView([]byte{}))
	})
	t.Run("zeroAlloc", func(t *testing.T) {
		b := []byte("test string")
		assert.Zero(t, testing.AllocsPerRun(50, func() { _ = BytesView(b) }))
	})
}

func TestMaskRanges(t *testing.T) {
	// No ranges (nil or empty): original slice returned unchanged,
	// with no allocation — the loop body never runs.
	in := []byte("abc")
	assert.Equal(t, in, MaskRanges(in, 0, nil))
	assert.Equal(t, in, MaskRanges(in, 0, []Range{}))
	assert.Zero(t, testing.AllocsPerRun(20, func() { _ = MaskRanges(in, 0, nil) }))
	// Range entirely before the line: skipped, original returned.
	assert.Equal(t, []byte("abc"),
		MaskRanges([]byte("abc"), 100, []Range{{Start: 0, End: 5}}))
	// Range overruns both ends: from clamps to 0, to clamps to len.
	assert.Equal(t, []byte("     "),
		MaskRanges([]byte("abcde"), 10, []Range{{Start: 8, End: 30}}))
	// Range within the line: only the overlap is blanked.
	assert.Equal(t, []byte("ab cd"),
		MaskRanges([]byte("abXcd"), 0, []Range{{Start: 2, End: 3}}))
}
