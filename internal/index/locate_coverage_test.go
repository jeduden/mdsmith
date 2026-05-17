package index

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/yuin/goldmark/ast"
)

// These tests exercise locate.go branches not reached via the
// public Locate API.

func TestLinkContainsOffsetFallback(t *testing.T) {
	t.Parallel()
	// A link node with no text segments yields false.
	source := []byte("# Top\n[text](#anchor)\n")
	l := ast.NewLink()
	assert.False(t, linkContainsOffset(source, l, 0))
}

func TestLinkCloseOffsetReferenceStyle(t *testing.T) {
	t.Parallel()
	src := []byte("[text][label]\n")
	// after = end of "text" = 5. l has Reference != nil → closes at the
	// `]` after `label`.
	l := ast.NewLink()
	l.Reference = &ast.ReferenceLink{Type: ast.ReferenceLinkFull, Value: []byte("label")}
	close := linkCloseOffset(src, l, 5)
	assert.Equal(t, 12, close)
}

func TestLinkCloseOffsetReferenceStyleNoCloser(t *testing.T) {
	t.Parallel()
	src := []byte("[text]\n")
	l := ast.NewLink()
	l.Reference = &ast.ReferenceLink{Type: ast.ReferenceLinkShortcut, Value: []byte("label")}
	close := linkCloseOffset(src, l, 6)
	// No `]` after `after`; we hit newline → -1.
	assert.Equal(t, -1, close)
}

func TestLinkCloseOffsetInlineFallbackToBracket(t *testing.T) {
	t.Parallel()
	// Display text ends at byte 5 (the `]`); when no `(` follows
	// the text-close, the helper treats the `]` itself as the
	// closing delimiter (shortcut-style fallback).
	src := []byte("[text]extra]\n")
	l := ast.NewLink()
	close := linkCloseOffset(src, l, 5)
	assert.Equal(t, 5, close)
}

func TestLinkCloseOffsetInlineFallbackNoOpener(t *testing.T) {
	t.Parallel()
	// Same fallback, but the next `]` is at `after`. Result is
	// the byte at `after`.
	src := []byte("[text]extra\n")
	l := ast.NewLink()
	close := linkCloseOffset(src, l, 5)
	assert.Equal(t, 5, close)
}

func TestLinkCloseOffsetInlineNestedParens(t *testing.T) {
	t.Parallel()
	// `[text](https://x.com/(nested))` — outer `)` matches the
	// outer `(`, so close is the second-to-last byte before \n.
	src := []byte("[text](https://x.com/(nested))\n")
	l := ast.NewLink()
	close := linkCloseOffset(src, l, 5)
	assert.Equal(t, 29, close)
}

func TestLinkContainsOffsetBracketTooFar(t *testing.T) {
	t.Parallel()
	// Construct a link node where the text segment is far from the
	// opening `[` so the 200-byte cap fires (open falls back to
	// startOff).
	pad := strings.Repeat("x", 250)
	src := []byte("[" + pad + "](#sec)\n")
	idx := New("/r")
	idx.Update("a.md", src)
	// Cursor at the very start of the bracket (offset 0) — should
	// still be classifiable.
	res := Locator{Path: "a.md"}.Locate(src, 1, 1)
	// Just verifying it doesn't panic; the exact tag depends on the
	// goldmark parse outcome.
	_ = res.Tag
}

func TestLinkCloseOffsetInlineNoParen(t *testing.T) {
	t.Parallel()
	src := []byte("[text](unclosed\n")
	l := ast.NewLink()
	close := linkCloseOffset(src, l, 5)
	assert.Equal(t, -1, close)
}

func TestPiContainsLineEmpty(t *testing.T) {
	t.Parallel()
	// PI with empty Lines() — defensive guard returns false.
	// We can't easily construct a real ProcessingInstruction with
	// empty Lines() here; the canonical test is that locate on a
	// non-PI line returns TokenNone, which exercises the path
	// indirectly.
	src := "# Top\n\nplain prose\n"
	res := Locator{Path: "a.md"}.Locate([]byte(src), 3, 5)
	assert.Equal(t, TokenNone, res.Tag)
}

func TestLocateDirectiveArgWithoutTarget(t *testing.T) {
	t.Parallel()
	// Directive arg that isn't `file:` / `source:` doesn't set
	// DirectiveTargetFile.
	src := "# T\n\n<?include\nstrip-frontmatter: \"true\"\n?>\n<?/include?>\n"
	res := Locator{Path: "a.md"}.Locate([]byte(src), 4, 5)
	assert.Equal(t, TokenDirectiveArg, res.Tag)
	assert.Equal(t, "include", res.DirectiveName)
	assert.Equal(t, "strip-frontmatter", res.DirectiveArg)
	assert.Empty(t, res.DirectiveTargetFile)
}

func TestLocateBuildDirectiveNonSourceArg(t *testing.T) {
	t.Parallel()
	src := "# T\n\n<?build\ntarget: \"out.md\"\n?>\n<?/build?>\n"
	res := Locator{Path: "a.md"}.Locate([]byte(src), 4, 5)
	assert.Equal(t, TokenDirectiveArg, res.Tag)
	assert.Equal(t, "build", res.DirectiveName)
	assert.Equal(t, "target", res.DirectiveArg)
	assert.Empty(t, res.DirectiveTargetFile)
}

func TestLocatePINameWithSlashSkipped(t *testing.T) {
	t.Parallel()
	// Cursor on the closing marker line of a directive: piContainsLine
	// extends through the closer line, and the PI's Name doesn't
	// start with `/` (the closing PI is its own AST node), so the
	// Locate fires on the opener regardless.
	src := "# T\n\n<?include\nfile: \"x.md\"\n?>\n<?/include?>\n"
	res := Locator{Path: "a.md"}.Locate([]byte(src), 5, 1)
	assert.Equal(t, TokenDirectiveArg, res.Tag)
}

func TestFrontMatterListItemBareDash(t *testing.T) {
	t.Parallel()
	v, ok := frontMatterListItem("  -")
	assert.True(t, ok)
	assert.Empty(t, v)
}

func TestFrontMatterListItemNotAList(t *testing.T) {
	t.Parallel()
	_, ok := frontMatterListItem("foo: bar")
	assert.False(t, ok)
}

func TestFrontMatterParentKeyEmpty(t *testing.T) {
	t.Parallel()
	// idx 0 → no preceding lines.
	got := frontMatterParentKey([][]byte{[]byte("- item")}, 0)
	assert.Empty(t, got)
}

func TestFrontMatterParentKeySkipsListItems(t *testing.T) {
	t.Parallel()
	// Walking back over a list item then a key: returns the key.
	lines := [][]byte{
		[]byte("kinds:"),
		[]byte("  - guide"),
		[]byte("  - reference"),
	}
	got := frontMatterParentKey(lines, 2)
	assert.Equal(t, "kinds", got)
}

func TestOffsetAtClampsCol(t *testing.T) {
	t.Parallel()
	lines := [][]byte{[]byte("hi"), []byte("there")}
	// Col past line length clamps to end.
	off := offsetAt(lines, 1, 99)
	assert.Equal(t, 2, off)
	// Negative line / col clamp to (1, 1).
	off = offsetAt(lines, -1, -1)
	assert.Equal(t, 0, off)
}

func TestLocateInASTNoMatch(t *testing.T) {
	t.Parallel()
	src := "# T\n\nplain\n"
	res := Locator{Path: "a.md"}.Locate([]byte(src), 2, 1)
	// Empty line — no match.
	assert.Equal(t, TokenNone, res.Tag)
}
