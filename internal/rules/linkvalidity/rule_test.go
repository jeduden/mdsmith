package linkvalidity

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
)

func check(t *testing.T, src string) []lint.Diagnostic {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	return (&Rule{}).Check(f)
}

func fix(t *testing.T, src string) string {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	return string((&Rule{}).Fix(f))
}

func TestIDNameCategory(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "MDS062", r.ID())
	assert.Equal(t, "link-validity", r.Name())
	assert.Equal(t, "link", r.Category())
}

// --- MD011: reversed links ---

func TestReversedLinkFlagged(t *testing.T) {
	diags := check(t, "# T\n\n(text)[url] here.\n")
	require.Len(t, diags, 1)
	assert.Equal(t, 3, diags[0].Line)
	assert.Equal(t, 1, diags[0].Column)
	assert.Equal(t, "reversed link: use [text](url) instead of (text)[url]", diags[0].Message)
}

func TestReversedLinkColumn(t *testing.T) {
	diags := check(t, "# T\n\nsee (the docs)[https://example.com] now\n")
	require.Len(t, diags, 1)
	assert.Equal(t, 3, diags[0].Line)
	assert.Equal(t, 5, diags[0].Column)
}

func TestReversedLinkFixed(t *testing.T) {
	got := fix(t, "# T\n\n(text)[https://example.com] x\n")
	assert.Equal(t, "# T\n\n[text](https://example.com) x\n", got)
}

func TestTwoReversedOnOneLine(t *testing.T) {
	diags := check(t, "# T\n\n(a)[https://a.test] and (c)[https://c.test]\n")
	require.Len(t, diags, 2)
	assert.Equal(t, 1, diags[0].Column)
	assert.Equal(t, 25, diags[1].Column)
}

func TestReversedInCodeSpanIgnored(t *testing.T) {
	diags := check(t, "# T\n\nThe `(text)[url]` pattern is reversed.\n")
	assert.Empty(t, diags)
}

func TestReversedInFencedBlockIgnored(t *testing.T) {
	diags := check(t, "# T\n\n```\n(text)[url]\n```\n")
	assert.Empty(t, diags)
}

func TestReversedInIndentedCodeIgnored(t *testing.T) {
	diags := check(t, "# T\n\n    (text)[url]\n")
	assert.Empty(t, diags)
}

func TestAdjacentNormalLinksNotReversed(t *testing.T) {
	diags := check(t, "# T\n\n[a](https://a.test)[c](https://c.test)\n")
	assert.Empty(t, diags)
}

func TestEscapedParenNotReversed(t *testing.T) {
	diags := check(t, "# T\n\n\\(text)[url] literal\n")
	assert.Empty(t, diags)
}

func TestNormalLinkClean(t *testing.T) {
	diags := check(t, "# T\n\n[text](https://example.com) is fine.\n")
	assert.Empty(t, diags)
}

func TestAutolinkClean(t *testing.T) {
	diags := check(t, "# T\n\nVisit <https://example.com> today.\n")
	assert.Empty(t, diags)
}

func TestReversedInDirectiveBodyIgnored(t *testing.T) {
	src := "# T\n\n<?include\nfile: x.md\n?>\n(text)[url]\n<?/include?>\n"
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	f.GeneratedRanges = []lint.LineRange{{From: 6, To: 6}}
	diags := (&Rule{}).Check(f)
	assert.Empty(t, diags)
}

func TestReversedOnPIMarkerLineIgnored(t *testing.T) {
	// A reversed-looking token inside a directive header line is raw and
	// must not be flagged.
	src := "# T\n\n<?catalog\nrow: \"(name)[link]\"\n?>\n<?/catalog?>\n"
	diags := check(t, src)
	assert.Empty(t, diags)
}

func TestFixLeavesCodeSpanReversedAlone(t *testing.T) {
	src := "# T\n\nKeep `(x)[y]` but fix (a)[https://a.test].\n"
	got := fix(t, src)
	assert.Equal(t, "# T\n\nKeep `(x)[y]` but fix [a](https://a.test).\n", got)
}

func TestFixNoChange(t *testing.T) {
	src := "# T\n\nNothing to do here.\n"
	assert.Equal(t, src, fix(t, src))
}

// --- MD042: empty links ---

func TestEmptyDestinationFlagged(t *testing.T) {
	diags := check(t, "# T\n\n[text]() is broken.\n")
	require.Len(t, diags, 1)
	assert.Equal(t, 3, diags[0].Line)
	assert.Equal(t, "empty link destination", diags[0].Message)
}

func TestEmptyTextFlagged(t *testing.T) {
	diags := check(t, "# T\n\n[](https://example.com) is broken.\n")
	require.Len(t, diags, 1)
	assert.Equal(t, "empty link text", diags[0].Message)
}

func TestWhitespaceDestinationAndText(t *testing.T) {
	diags := check(t, "# T\n\n[ ]( ) is broken.\n")
	// Whitespace-only destination is reported; text is whitespace-only too
	// but destination is checked first and is the actionable defect.
	require.Len(t, diags, 1)
	assert.Equal(t, "empty link destination", diags[0].Message)
}

func TestHashOnlyDestinationFlagged(t *testing.T) {
	diags := check(t, "# T\n\n[frag](#) goes nowhere.\n")
	require.Len(t, diags, 1)
	assert.Equal(t, "empty link destination", diags[0].Message)
}

func TestFragmentDestinationClean(t *testing.T) {
	diags := check(t, "# T\n\n[section](#section) is fine.\n")
	assert.Empty(t, diags)
}

func TestEmptyImageDestinationFlagged(t *testing.T) {
	diags := check(t, "# T\n\n![alt]() is broken.\n")
	require.Len(t, diags, 1)
	assert.Equal(t, "empty image destination", diags[0].Message)
}

func TestImageWithSourceClean(t *testing.T) {
	diags := check(t, "# T\n\n![alt](img.png) is fine.\n")
	assert.Empty(t, diags)
}

func TestEmptyAltImageNotFlaggedHere(t *testing.T) {
	// Empty alt text with a valid destination is MDS032's concern,
	// not link-validity's.
	diags := check(t, "# T\n\n![](img.png)\n")
	assert.Empty(t, diags)
}

func TestLinkedImageClean(t *testing.T) {
	diags := check(t, "# T\n\n[![logo](logo.png)](https://example.com)\n")
	assert.Empty(t, diags)
}

func TestEmptyLinkNotAutofixed(t *testing.T) {
	src := "# T\n\n[text]() stays.\n"
	assert.Equal(t, src, fix(t, src))
}

func TestEmptyLinkInCodeSpanClean(t *testing.T) {
	diags := check(t, "# T\n\nThe `[text]()` syntax is empty.\n")
	assert.Empty(t, diags)
}

func TestDiagnosticsSortedByLine(t *testing.T) {
	src := "# T\n\n[](https://example.com)\n\n(text)[url]\n"
	diags := check(t, src)
	require.Len(t, diags, 2)
	assert.Equal(t, 3, diags[0].Line)
	assert.Equal(t, 5, diags[1].Line)
}

func TestTwoDiagsSameLineSortedByColumn(t *testing.T) {
	// An empty-text link and a reversed link on the same line exercise
	// the comparator's line-equal, compare-by-column path.
	diags := check(t, "# T\n\n[](https://example.com) then (a)[https://b.test]\n")
	require.Len(t, diags, 2)
	assert.Equal(t, 3, diags[0].Line)
	assert.Equal(t, 3, diags[1].Line)
	assert.Less(t, diags[0].Column, diags[1].Column)
}

func TestEmptyLinkInsideEmphasis(t *testing.T) {
	// nodeLine must skip the inline Emphasis ancestor and resolve the
	// line from the enclosing paragraph.
	diags := check(t, "# T\n\nlead\n\n*[](https://example.com)*\n")
	require.Len(t, diags, 1)
	assert.Equal(t, "empty link text", diags[0].Message)
	assert.Equal(t, 5, diags[0].Line)
}

func TestEmptyDestLinkWithEmphasisText(t *testing.T) {
	// firstTextLine recurses past the Emphasis wrapper to the text run.
	diags := check(t, "# T\n\nan [*here*]() link\n")
	require.Len(t, diags, 1)
	assert.Equal(t, "empty link destination", diags[0].Message)
	assert.Equal(t, 3, diags[0].Line)
}

func TestNodeLineDetachedFallback(t *testing.T) {
	f, err := lint.NewFile("t.md", []byte("x"))
	require.NoError(t, err)
	assert.Equal(t, 1, nodeLine(ast.NewLink(), f))
}

func TestFixSkipsFencedBlock(t *testing.T) {
	src := "# T\n\n```text\n(a)[b]\n```\n"
	assert.Equal(t, src, fix(t, src))
}

func TestFixSkipsGeneratedRange(t *testing.T) {
	src := "# T\n\n<?include\nfile: x.md\n?>\n(a)[b]\n<?/include?>\n"
	f, err := lint.NewFile("t.md", []byte(src))
	require.NoError(t, err)
	f.GeneratedRanges = []lint.LineRange{{From: 6, To: 6}}
	assert.Equal(t, src, string((&Rule{}).Fix(f)))
}

func TestReversedFollowingParenGuard(t *testing.T) {
	// '(bar)[baz]' is followed by '(', so it is the normal link
	// [baz](https://q.test), not a reversed link.
	diags := check(t, "# T\n\nfoo (bar)[baz](https://q.test) end\n")
	assert.Empty(t, diags)
}

func TestReversedInLineGuardsDirect(t *testing.T) {
	none := func(s string) {
		t.Helper()
		assert.Empty(t, reversedInLine([]byte(s), []byte(s)), s)
	}
	none(`\(a)[b]`)   // escaped '('
	none(`x](a)[b]`)  // preceding ']' — real link destination
	none(`(a)[b](c)`) // following '(' — normal link
	got := reversedInLine([]byte(`(a)[b]`), []byte(`(a)[b]`))
	require.Len(t, got, 1)
	assert.Equal(t, "a", string(got[0].text))
	assert.Equal(t, "b", string(got[0].url))
}

func TestMaskLineDirect(t *testing.T) {
	// No ranges: original slice returned unchanged.
	assert.Equal(t, []byte("abc"), maskLine([]byte("abc"), 0, nil))
	// Range entirely before the line: skipped, original returned.
	assert.Equal(t, []byte("abc"),
		maskLine([]byte("abc"), 100, []byteRange{{0, 5}}))
	// Range overruns both ends: from clamps to 0, to clamps to len.
	assert.Equal(t, []byte("     "),
		maskLine([]byte("abcde"), 10, []byteRange{{8, 30}}))
	// Range within the line: only the overlap is blanked.
	assert.Equal(t, []byte("ab cd"),
		maskLine([]byte("abXcd"), 0, []byteRange{{2, 3}}))
}

func TestMultiLineCodeSpanNotFlagged(t *testing.T) {
	// A code span whose content spans two lines must still mask the
	// reversed shape on both lines.
	diags := check(t, "# T\n\nuse `(a)\n[b]` here\n")
	assert.Empty(t, diags)
}
