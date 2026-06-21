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
