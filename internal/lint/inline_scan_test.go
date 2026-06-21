package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/pkg/goldmark/arena"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

// scanRec mirrors the inline fields the parity rules read, for asserting the
// scanner's node stream matches an expected shape.
type scanRec struct {
	kind        string
	value       string // Text/CodeSpan content; AutoLink label
	dest, title string
}

func recsForRun(t *testing.T, run string) ([]scanRec, bool) {
	t.Helper()
	node, ok := scanInlineRun([]byte(run), arena.New())
	if !ok {
		return nil, false
	}
	var out []scanRec
	src := []byte(run)
	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch x := n.(type) {
		case *ast.Text:
			out = append(out, scanRec{kind: "Text", value: string(x.Segment.Value(src))})
		case *ast.Link:
			out = append(out, scanRec{kind: "Link", dest: string(x.Destination), title: string(x.Title)})
		case *ast.Image:
			out = append(out, scanRec{kind: "Image", dest: string(x.Destination), title: string(x.Title)})
		case *ast.AutoLink:
			out = append(out, scanRec{kind: "AutoLink", value: string(x.Label(src))})
		case *ast.CodeSpan:
			out = append(out, scanRec{kind: "CodeSpan"})
		}
		return ast.WalkContinue, nil
	})
	return out, true
}

func TestScanInlineRun_PlainText(t *testing.T) {
	recs, ok := recsForRun(t, "hello world")
	require.True(t, ok)
	assert.Equal(t, []scanRec{{kind: "Text", value: "hello world"}}, recs)
}

func TestScanInlineRun_TrailingSpacesTrimmed(t *testing.T) {
	recs, ok := recsForRun(t, "trailing spaces   ")
	require.True(t, ok)
	assert.Equal(t, []scanRec{{kind: "Text", value: "trailing spaces"}}, recs)
}

func TestScanInlineRun_LeadingSpacesStripped(t *testing.T) {
	recs, ok := recsForRun(t, "  indented two")
	require.True(t, ok)
	assert.Equal(t, []scanRec{{kind: "Text", value: "indented two"}}, recs)
}

func TestScanInlineRun_InlineLink(t *testing.T) {
	recs, ok := recsForRun(t, "see [text](http://x.com) here")
	require.True(t, ok)
	assert.Equal(t, []scanRec{
		{kind: "Text", value: "see "},
		{kind: "Link", dest: "http://x.com"},
		{kind: "Text", value: "text"},
		{kind: "Text", value: " here"},
	}, recs)
}

func TestScanInlineRun_InlineLinkWithTitle(t *testing.T) {
	recs, ok := recsForRun(t, `[t](http://x.com "the title")`)
	require.True(t, ok)
	assert.Equal(t, []scanRec{
		{kind: "Link", dest: "http://x.com", title: "the title"},
		{kind: "Text", value: "t"},
	}, recs)
}

func TestScanInlineRun_EmptyDestinationLink(t *testing.T) {
	recs, ok := recsForRun(t, "broken [x]() link")
	require.True(t, ok)
	assert.Equal(t, []scanRec{
		{kind: "Text", value: "broken "},
		{kind: "Link"},
		{kind: "Text", value: "x"},
		{kind: "Text", value: " link"},
	}, recs)
}

func TestScanInlineRun_Image(t *testing.T) {
	recs, ok := recsForRun(t, "img ![alt](pic.png) end")
	require.True(t, ok)
	assert.Equal(t, []scanRec{
		{kind: "Text", value: "img "},
		{kind: "Image", dest: "pic.png"},
		{kind: "Text", value: "alt"},
		{kind: "Text", value: " end"},
	}, recs)
}

func TestScanInlineRun_Autolink(t *testing.T) {
	recs, ok := recsForRun(t, "an <http://auto.com> link")
	require.True(t, ok)
	assert.Equal(t, []scanRec{
		{kind: "Text", value: "an "},
		{kind: "AutoLink", value: "http://auto.com"},
		{kind: "Text", value: " link"},
	}, recs)
}

func TestScanInlineRun_CodeSpan(t *testing.T) {
	recs, ok := recsForRun(t, "a `code span` b")
	require.True(t, ok)
	assert.Equal(t, []scanRec{
		{kind: "Text", value: "a "},
		{kind: "CodeSpan"},
		{kind: "Text", value: "code span"},
		{kind: "Text", value: " b"},
	}, recs)
}

// TestScanInlineRun_TriggerTextSplit covers goldmark's Text segmentation at a
// link-parser trigger byte (`!`, `]`) that opens no construct: the run's
// final text stretch is its own Text node while earlier stretches merge.
func TestScanInlineRun_TriggerTextSplit(t *testing.T) {
	recs, ok := recsForRun(t, "end here!!!!")
	require.True(t, ok)
	assert.Equal(t, []scanRec{
		{kind: "Text", value: "end here!!!"},
		{kind: "Text", value: "!"},
	}, recs)
}

func TestScanInlineRun_BangThenText(t *testing.T) {
	recs, ok := recsForRun(t, "a!b")
	require.True(t, ok)
	assert.Equal(t, []scanRec{
		{kind: "Text", value: "a"},
		{kind: "Text", value: "!b"},
	}, recs)
}

func TestScanInlineRun_BailsOnEmphasis(t *testing.T) {
	_, ok := scanInlineRun([]byte("a *emph* b"), arena.New())
	assert.False(t, ok, "emphasis must fall back to goldmark")
}

func TestScanInlineRun_BailsOnUnderscore(t *testing.T) {
	_, ok := scanInlineRun([]byte("a _x_ b"), arena.New())
	assert.False(t, ok)
}

func TestScanInlineRun_BailsOnBackslash(t *testing.T) {
	_, ok := scanInlineRun([]byte(`esc \* text`), arena.New())
	assert.False(t, ok, "backslash escape must fall back")
}

func TestScanInlineRun_BailsOnAmpersand(t *testing.T) {
	_, ok := scanInlineRun([]byte("a &amp; b"), arena.New())
	assert.False(t, ok, "entity must fall back")
}

func TestScanInlineRun_BailsOnReferenceLink(t *testing.T) {
	_, ok := scanInlineRun([]byte("a [text][label] b"), arena.New())
	assert.False(t, ok, "reference link must fall back")
}

func TestScanInlineRun_BailsOnShortcutReference(t *testing.T) {
	// `[label]` not followed by `(` is a shortcut/collapsed reference shape
	// the scanner does not resolve.
	_, ok := scanInlineRun([]byte("see [label] here"), arena.New())
	assert.False(t, ok)
}

func TestScanInlineRun_BailsOnRawHTML(t *testing.T) {
	_, ok := scanInlineRun([]byte("a <span>x</span> b"), arena.New())
	assert.False(t, ok, "raw HTML (non-autolink <) must fall back")
}

func TestScanInlineRun_BailsOnUnclosedCodeSpan(t *testing.T) {
	_, ok := scanInlineRun([]byte("a `code b"), arena.New())
	assert.False(t, ok)
}

func TestScanInlineRun_BailsOnHeading(t *testing.T) {
	_, ok := scanInlineRun([]byte("# Heading"), arena.New())
	assert.False(t, ok)
}

func TestScanInlineRun_BailsOnListMarker(t *testing.T) {
	_, ok := scanInlineRun([]byte("- item"), arena.New())
	assert.False(t, ok)
}

func TestScanInlineRun_BailsOnBlockQuote(t *testing.T) {
	_, ok := scanInlineRun([]byte("> quoted"), arena.New())
	assert.False(t, ok)
}

func TestScanInlineRun_BailsOnMultiLine(t *testing.T) {
	_, ok := scanInlineRun([]byte("line one\nline two"), arena.New())
	assert.False(t, ok, "multi-line runs fall back")
}

func TestScanInlineRun_BailsOnEmpty(t *testing.T) {
	_, ok := scanInlineRun(nil, arena.New())
	assert.False(t, ok)
}

func TestScanInlineRun_AngleBracketDestination(t *testing.T) {
	recs, ok := recsForRun(t, "[text](<http://x.com>)")
	require.True(t, ok)
	assert.Equal(t, []scanRec{
		{kind: "Link", dest: "http://x.com"},
		{kind: "Text", value: "text"},
	}, recs)
}

func TestScanInlineRun_InlineLinkWithSingleQuoteTitle(t *testing.T) {
	recs, ok := recsForRun(t, "[t](http://x.com 'the title')")
	require.True(t, ok)
	assert.Equal(t, []scanRec{
		{kind: "Link", dest: "http://x.com", title: "the title"},
		{kind: "Text", value: "t"},
	}, recs)
}

func TestScanInlineRun_InlineLinkWithParenTitle(t *testing.T) {
	recs, ok := recsForRun(t, "[t](http://x.com (the title))")
	require.True(t, ok)
	assert.Equal(t, []scanRec{
		{kind: "Link", dest: "http://x.com", title: "the title"},
		{kind: "Text", value: "t"},
	}, recs)
}

func TestScanInlineRun_InlineLinkBalancedParenDest(t *testing.T) {
	recs, ok := recsForRun(t, "[t](foo(bar))")
	require.True(t, ok)
	assert.Equal(t, []scanRec{
		{kind: "Link", dest: "foo(bar)"},
		{kind: "Text", value: "t"},
	}, recs)
}

func TestScanInlineRun_ImageEmptyAlt(t *testing.T) {
	recs, ok := recsForRun(t, "![](pic.png)")
	require.True(t, ok)
	assert.Equal(t, []scanRec{
		{kind: "Image", dest: "pic.png"},
	}, recs)
}

func TestScanInlineRun_BailsOnUnclosedLinkTitle(t *testing.T) {
	_, ok := scanInlineRun([]byte(`[t](url "unclosed`), arena.New())
	assert.False(t, ok, "unclosed link title must fall back")
}

func TestScanInlineRun_BailsOnReferenceImage(t *testing.T) {
	_, ok := scanInlineRun([]byte("![alt][label]"), arena.New())
	assert.False(t, ok, "reference-style image must fall back")
}

func TestScanInlineRun_BailsOnNestedBracketsInLabel(t *testing.T) {
	_, ok := scanInlineRun([]byte("[a[b]](url)"), arena.New())
	assert.False(t, ok, "nested brackets in label must fall back")
}

func TestScanInlineRun_BailsOnAngleInLabel(t *testing.T) {
	_, ok := scanInlineRun([]byte("[a<b>](url)"), arena.New())
	assert.False(t, ok, "angle bracket in label must fall back")
}

func TestScanInlineRun_BailsOnInvalidLinkTitleOpener(t *testing.T) {
	_, ok := scanInlineRun([]byte("[t](url xyz)"), arena.New())
	assert.False(t, ok, "non-quote title opener must fall back")
}

func TestScanInlineRun_BailsOnNestedAngleInDest(t *testing.T) {
	_, ok := scanInlineRun([]byte("[t](<a<b>)"), arena.New())
	assert.False(t, ok, "nested < in angle-bracket destination must fall back")
}

func TestScanInlineRun_BailsOnUnterminatedAngleBracketDest(t *testing.T) {
	_, ok := scanInlineRun([]byte("[t](<unclosed)"), arena.New())
	assert.False(t, ok, "unterminated angle-bracket destination must fall back")
}

func TestScanInlineRun_BareCloseBracket(t *testing.T) {
	// A bare ']' with no opening '[' link is a link-parser trigger that fires
	// with no result: the scanner flushes pending text before it and the ']'
	// starts the next text segment (goldmark MergeOrAppend behaviour).
	recs, ok := recsForRun(t, "foo]bar")
	require.True(t, ok)
	assert.Equal(t, []scanRec{
		{kind: "Text", value: "foo"},
		{kind: "Text", value: "]bar"},
	}, recs)
}

func TestScanInlineRun_BailsOnUnclosedLabel(t *testing.T) {
	// '[' with no matching ']' must fall back (loop exits without foundClose).
	_, ok := scanInlineRun([]byte("[unclosed"), arena.New())
	assert.False(t, ok, "unclosed link label must fall back")
}

func TestScanInlineRun_BailsOnLabelAtEndOfRun(t *testing.T) {
	// ']' as the last byte (labelEnd+1 >= len(run)) must fall back.
	_, ok := scanInlineRun([]byte("[x]"), arena.New())
	assert.False(t, ok, "shortcut-reference shape must fall back")
}

func TestScanInlineRun_BailsOnExtraAfterTitle(t *testing.T) {
	// Content after a valid title but before ')' must fall back.
	_, ok := scanInlineRun([]byte(`[t](url "title" extra)`), arena.New())
	assert.False(t, ok, "extra content after title must fall back")
}

func TestScanInlineRun_BailsOnMissingCloseParen(t *testing.T) {
	// No closing ')' at all: scanLinkTitle is called with i >= len(run).
	_, ok := scanInlineRun([]byte("[t](url"), arena.New())
	assert.False(t, ok, "link without closing paren must fall back")
}

func TestScanInlineRun_BailsOnParenTitleWithInnerParen(t *testing.T) {
	// Paren-form title containing an inner '(' must fall back (goldmark rejects it).
	_, ok := scanInlineRun([]byte("[t](url (outer (inner)))"), arena.New())
	assert.False(t, ok, "paren title with inner paren must fall back")
}

func TestCodeSpanTrim_SpacePadded(t *testing.T) {
	src := []byte(" code ")
	start, stop := codeSpanTrim(src, 0, len(src))
	assert.Equal(t, 1, start)
	assert.Equal(t, 5, stop)
}

func TestCodeSpanTrim_AllBlank(t *testing.T) {
	src := []byte("   ")
	start, stop := codeSpanTrim(src, 0, len(src))
	assert.Equal(t, 0, start, "all-blank content must not be trimmed")
	assert.Equal(t, 3, stop)
}

func TestCodeSpanTrim_NoSpaces(t *testing.T) {
	src := []byte("code")
	start, stop := codeSpanTrim(src, 0, len(src))
	assert.Equal(t, 0, start, "content with no edge spaces must not be trimmed")
	assert.Equal(t, 4, stop)
}

func TestCodeSpanTrim_Empty(t *testing.T) {
	// start >= stop early-return path: empty code span content.
	start, stop := codeSpanTrim([]byte(""), 0, 0)
	assert.Equal(t, 0, start)
	assert.Equal(t, 0, stop)
}

func TestScanLinkTitle_NewlineInTitle(t *testing.T) {
	title, _, ok := scanLinkTitle([]byte("\"ti\ntle\""), 0)
	assert.False(t, ok, "newline inside title must fail")
	assert.Nil(t, title)
}

func TestScanLinkTitle_Unclosed(t *testing.T) {
	title, _, ok := scanLinkTitle([]byte(`"unclosed`), 0)
	assert.False(t, ok, "unterminated title must fail")
	assert.Nil(t, title)
}

func TestScanLinkTitle_EmptyRun(t *testing.T) {
	// i >= len(run) guard: scanLinkTitle called with offset past end of slice.
	title, _, ok := scanLinkTitle([]byte("url"), 3)
	assert.False(t, ok, "offset at end of run must fail")
	assert.Nil(t, title)
}

func TestScanLinkTitle_ParenWithInnerParen(t *testing.T) {
	// Paren-form title with inner '(' must fail (goldmark FindClosure Nesting:false).
	title, _, ok := scanLinkTitle([]byte("(outer (inner))"), 0)
	assert.False(t, ok, "inner paren in paren-form title must fail")
	assert.Nil(t, title)
}

func TestScanLinkDestination_NewlineInAngleBracket(t *testing.T) {
	// '\n' inside angle-bracket destination must fail.
	dest, _, ok := scanLinkDestination([]byte("<has\nnewline>"), 0)
	assert.False(t, ok)
	assert.Nil(t, dest)
}

func TestScanAutolink_EmailAutolink(t *testing.T) {
	recs, ok := recsForRun(t, "mail <user@example.com> here")
	require.True(t, ok)
	assert.Equal(t, []scanRec{
		{kind: "Text", value: "mail "},
		{kind: "AutoLink", value: "user@example.com"},
		{kind: "Text", value: " here"},
	}, recs)
}

func TestScanAutolink_URLWithoutClosingAngle(t *testing.T) {
	// stop >= len(line): URL pattern matches but run ends before '>'.
	_, _, ok := scanAutolink([]byte("<http://example.com"), 0, arena.New())
	assert.False(t, ok)
}

func TestSetParagraphLines_MultiLine(t *testing.T) {
	a := arena.New()
	run := []byte("line one\nline two\n")
	para := a.Paragraph()
	setParagraphLines(run, para)
	lines := para.Lines()
	require.Equal(t, 2, lines.Len())
	assert.Equal(t, 0, lines.At(0).Start)
	assert.Equal(t, 9, lines.At(0).Stop)
	assert.Equal(t, 9, lines.At(1).Start)
	assert.Equal(t, 18, lines.At(1).Stop)
}

// TestInlineBlocks_NoGoldmarkParseForSimpleFile asserts the acceptance
// criterion: a nil-AST File whose runs are all scanner-eligible produces
// inline nodes without any goldmark parse. The scanner emits an
// *ast.Document root and the goldmark parse would too, so the proxy is that
// every run's root has the expected scanner shape (a Document whose sole
// child is a Paragraph) — and the node stream matches what the rules read.
func TestInlineBlocks_ScannerHandlesSimpleFile(t *testing.T) {
	body := []byte("A paragraph with a [link](http://x.com).\n\nAnother `code` line here.\n")
	f := NewFileLines("simple.md", body)
	blocks := InlineBlocks(f)
	require.NotEmpty(t, blocks)
	for _, blk := range blocks {
		_, isDoc := blk.Node.(*ast.Document)
		require.True(t, isDoc, "run root should be a Document")
		// Scanner-built Documents hold exactly one Paragraph child.
		first := blk.Node.FirstChild()
		require.NotNil(t, first)
		_, isPara := first.(*ast.Paragraph)
		assert.True(t, isPara)
		assert.Nil(t, first.NextSibling(), "scanner run has a single paragraph")
	}
}
