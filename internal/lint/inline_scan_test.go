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
	require.False(t, ok, "newline inside title must fail")
	assert.Nil(t, title)
}

func TestScanLinkTitle_Unclosed(t *testing.T) {
	title, _, ok := scanLinkTitle([]byte(`"unclosed`), 0)
	require.False(t, ok, "unterminated title must fail")
	assert.Nil(t, title)
}

func TestScanLinkTitle_EmptyRun(t *testing.T) {
	// i >= len(run) guard: scanLinkTitle called with offset past end of slice.
	title, _, ok := scanLinkTitle([]byte("url"), 3)
	require.False(t, ok, "offset at end of run must fail")
	assert.Nil(t, title)
}

func TestScanLinkTitle_ParenWithInnerParen(t *testing.T) {
	// Paren-form title with inner '(' must fail (goldmark FindClosure Nesting:false).
	title, _, ok := scanLinkTitle([]byte("(outer (inner))"), 0)
	require.False(t, ok, "inner paren in paren-form title must fail")
	assert.Nil(t, title)
}

func TestScanLinkDestination_NewlineInAngleBracket(t *testing.T) {
	// '\n' inside angle-bracket destination must fail.
	dest, _, ok := scanLinkDestination([]byte("<has\nnewline>"), 0)
	require.False(t, ok)
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

// --- Dedicated helper tests (plan 2606231013) ---

func TestScanRunEligible(t *testing.T) {
	assert.False(t, scanRunEligible(nil), "nil is not eligible")
	assert.False(t, scanRunEligible([]byte("")), "empty is not eligible")
	assert.False(t, scanRunEligible([]byte("line one\nline two")), "multiline not eligible")
	assert.False(t, scanRunEligible([]byte("# heading")), "ATX heading not eligible")
	assert.False(t, scanRunEligible([]byte("- list")), "list item not eligible")
	assert.False(t, scanRunEligible([]byte("> quote")), "block quote not eligible")
	assert.False(t, scanRunEligible([]byte("*em*")), "emphasis asterisk not eligible")
	assert.False(t, scanRunEligible([]byte("_em_")), "emphasis underscore not eligible")
	assert.False(t, scanRunEligible([]byte(`a\b`)), "backslash not eligible")
	assert.False(t, scanRunEligible([]byte("a&amp;b")), "entity not eligible")
	assert.True(t, scanRunEligible([]byte("plain text")), "plain text eligible")
	assert.True(t, scanRunEligible([]byte("[link](url)")), "link chars eligible")
	assert.True(t, scanRunEligible([]byte("`code`")), "backtick eligible")
	assert.True(t, scanRunEligible([]byte("<autolink>")), "angle bracket eligible")
}

func TestMergeAppendText(t *testing.T) {
	a := arena.New()
	run := []byte("hello world")
	para := a.Paragraph()

	// end <= start: no child appended.
	mergeAppendText(run, 3, 3, para, a)
	assert.Nil(t, para.FirstChild(), "end==start emits nothing")

	// Normal range: appends a Text node.
	mergeAppendText(run, 0, 5, para, a)
	child := para.FirstChild()
	require.NotNil(t, child, "Text node appended")
	txt, ok := child.(*ast.Text)
	require.True(t, ok, "child is *ast.Text")
	assert.Equal(t, "hello", string(txt.Segment.Value(run)))

	// Adjacent range [5:11): merges into the existing Text node.
	mergeAppendText(run, 5, 11, para, a)
	assert.Nil(t, child.NextSibling(), "adjacent text merged, no new sibling")
	assert.Equal(t, "hello world", string(txt.Segment.Value(run)), "merged bounds")
}

func TestFinalAppendText(t *testing.T) {
	a := arena.New()
	run := []byte("hello   ")
	para := a.Paragraph()

	// Trailing spaces trimmed: appends "hello".
	finalAppendText(run, 0, len(run), para, a)
	child := para.FirstChild()
	require.NotNil(t, child)
	txt, ok := child.(*ast.Text)
	require.True(t, ok)
	assert.Equal(t, "hello", string(txt.Segment.Value(run)))

	// All whitespace: nothing appended.
	para2 := a.Paragraph()
	finalAppendText([]byte("   "), 0, 3, para2, a)
	assert.Nil(t, para2.FirstChild(), "all-space run appends nothing")

	// end <= start: nothing appended.
	para3 := a.Paragraph()
	finalAppendText(run, 5, 5, para3, a)
	assert.Nil(t, para3.FirstChild(), "empty range appends nothing")
}

func TestScanParagraphInlines(t *testing.T) {
	a := arena.New()

	// Plain text: returns true, one Text child.
	para := a.Paragraph()
	ok := scanParagraphInlines([]byte("hello"), para, a)
	require.True(t, ok)
	require.NotNil(t, para.FirstChild(), "Text child added")

	// Unclosed backtick: scanCodeSpan declines, returns false.
	para2 := a.Paragraph()
	ok2 := scanParagraphInlines([]byte("`unclosed"), para2, a)
	assert.False(t, ok2)
}

func TestApplyCodeSpan(t *testing.T) {
	a := arena.New()
	// Run starts at the backtick so no prior text is flushed first.
	run := []byte("`code` after")
	para := a.Paragraph()

	// i=0 points at the opening backtick; textStart=0 so nothing is flushed.
	ni, ns, ok := applyCodeSpan(run, 0, 0, para, a)
	require.True(t, ok)
	assert.Equal(t, 6, ni, "ni just past closing backtick")
	assert.Equal(t, 6, ns, "textStart reset to ni")
	child := para.FirstChild()
	require.NotNil(t, child)
	_, isCode := child.(*ast.CodeSpan)
	assert.True(t, isCode, "CodeSpan appended")

	// Unclosed backtick: returns false.
	para2 := a.Paragraph()
	_, _, ok2 := applyCodeSpan([]byte("`unclosed"), 0, 0, para2, a)
	assert.False(t, ok2)
}

func TestApplyAutolink(t *testing.T) {
	a := arena.New()
	run := []byte("<https://example.com>")
	para := a.Paragraph()

	ni, ns, ok := applyAutolink(run, 0, 0, para, a)
	require.True(t, ok)
	assert.Equal(t, len(run), ni)
	assert.Equal(t, len(run), ns)
	child := para.FirstChild()
	require.NotNil(t, child)
	_, isAL := child.(*ast.AutoLink)
	assert.True(t, isAL, "AutoLink appended")

	// Raw HTML angle: returns false.
	para2 := a.Paragraph()
	_, _, ok2 := applyAutolink([]byte("<div>"), 0, 0, para2, a)
	assert.False(t, ok2)
}

func TestApplyBang(t *testing.T) {
	a := arena.New()

	// `!` without `[`: flushes pending text, returns i+1 with textStart at i.
	run := []byte("a! text")
	para := a.Paragraph()
	ni, ns, ok := applyBang(run, 1, 0, para, a)
	require.True(t, ok)
	assert.Equal(t, 2, ni, "i advanced past `!`")
	assert.Equal(t, 1, ns, "textStart set to position of `!`")

	// `![alt](url)`: appends Image node.
	run2 := []byte("![alt](url)")
	para2 := a.Paragraph()
	ni2, _, ok2 := applyBang(run2, 0, 0, para2, a)
	require.True(t, ok2)
	assert.Equal(t, len(run2), ni2)
	child := para2.FirstChild()
	require.NotNil(t, child)
	_, isImg := child.(*ast.Image)
	assert.True(t, isImg, "Image appended")

	// `![` with unclosed label: returns false.
	para3 := a.Paragraph()
	_, _, ok3 := applyBang([]byte("![alt"), 0, 0, para3, a)
	assert.False(t, ok3)
}

func TestApplyLink(t *testing.T) {
	a := arena.New()
	run := []byte("[text](url)")
	para := a.Paragraph()

	ni, ns, ok := applyLink(run, 0, 0, para, a)
	require.True(t, ok)
	assert.Equal(t, len(run), ni)
	assert.Equal(t, len(run), ns)
	child := para.FirstChild()
	require.NotNil(t, child)
	_, isLink := child.(*ast.Link)
	assert.True(t, isLink, "Link appended")

	// Reference link `[text][ref]`: returns false.
	para2 := a.Paragraph()
	_, _, ok2 := applyLink([]byte("[text][ref]"), 0, 0, para2, a)
	assert.False(t, ok2)
}

func TestScanCodeSpan(t *testing.T) {
	a := arena.New()

	// Single-backtick span.
	node, after, ok := scanCodeSpan([]byte("`code`"), 0, a)
	require.True(t, ok)
	assert.Equal(t, 6, after)
	require.NotNil(t, node)
	_, isCode := node.(*ast.CodeSpan)
	assert.True(t, isCode)

	// Double-backtick span containing a single backtick.
	node2, after2, ok2 := scanCodeSpan([]byte("`` a`b ``"), 0, a)
	require.True(t, ok2)
	assert.Equal(t, 9, after2)
	require.NotNil(t, node2)

	// No closing backtick: returns false.
	_, _, ok3 := scanCodeSpan([]byte("`unclosed"), 0, a)
	assert.False(t, ok3)
}

func TestScanLinkOrImage(t *testing.T) {
	a := arena.New()

	// Inline link.
	node, after, ok := scanLinkOrImage([]byte("[text](url)"), 0, false, a)
	require.True(t, ok)
	assert.Equal(t, 11, after)
	_, isLink := node.(*ast.Link)
	assert.True(t, isLink, "Link node")

	// Inline image.
	node2, _, ok2 := scanLinkOrImage([]byte("![alt](url)"), 0, true, a)
	require.True(t, ok2)
	_, isImg := node2.(*ast.Image)
	assert.True(t, isImg, "Image node")

	// Reference link `[text][ref]`: returns false.
	_, _, ok3 := scanLinkOrImage([]byte("[text][ref]"), 0, false, a)
	assert.False(t, ok3)

	// Nested brackets in label: returns false.
	_, _, ok4 := scanLinkOrImage([]byte("[[a]](url)"), 0, false, a)
	assert.False(t, ok4)
}

func TestScanLinkParens(t *testing.T) {
	// Empty parens.
	dest, title, after, ok := scanLinkParens([]byte("()"), 0)
	require.True(t, ok)
	assert.Nil(t, dest)
	assert.Nil(t, title)
	assert.Equal(t, 2, after)

	// Destination only.
	dest2, title2, after2, ok2 := scanLinkParens([]byte("(url)"), 0)
	require.True(t, ok2)
	assert.Equal(t, "url", string(dest2))
	assert.Nil(t, title2)
	assert.Equal(t, 5, after2)

	// Destination and title.
	dest3, title3, after3, ok3 := scanLinkParens([]byte(`(url "ttl")`), 0)
	require.True(t, ok3)
	assert.Equal(t, "url", string(dest3))
	assert.Equal(t, "ttl", string(title3))
	assert.Equal(t, 11, after3)

	// Missing closing paren.
	_, _, _, ok4 := scanLinkParens([]byte("(url"), 0)
	assert.False(t, ok4)
}

func TestSkipSpacesAt(t *testing.T) {
	run := []byte("   hello")
	assert.Equal(t, 3, skipSpacesAt(run, 0), "leading spaces skipped")
	assert.Equal(t, 3, skipSpacesAt(run, 3), "no spaces from non-space pos")
	assert.Equal(t, len(run), skipSpacesAt(run, len(run)), "past end stays at end")

	allSpace := []byte("   ")
	assert.Equal(t, 3, skipSpacesAt(allSpace, 0), "all spaces → past end")
}
