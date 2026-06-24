package index

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/piparser"
	goldast "github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/goldmark/parser"
	"github.com/jeduden/mdsmith/pkg/goldmark/text"
)

func TestLocateHeading(t *testing.T) {
	t.Parallel()
	src := "# Top heading\n\nbody\n"
	res := Locator{Path: "a.md"}.Locate([]byte(src), 1, 3)
	assert.Equal(t, TokenHeading, res.Tag)
	assert.Equal(t, "top-heading", res.Anchor)
	assert.Equal(t, 1, res.Level)
}

func TestLocateAnchorLink(t *testing.T) {
	t.Parallel()
	src := "# Top\n\nSee [here](#sec).\n\n## Sec\n"
	// Cursor inside `[here](#sec)` (line 3, col 8).
	res := Locator{Path: "a.md"}.Locate([]byte(src), 3, 14)
	assert.Equal(t, TokenAnchorLink, res.Tag)
	assert.Equal(t, "sec", res.TargetAnchor)
}

func TestLocateFileLink(t *testing.T) {
	t.Parallel()
	src := "# Top\n\n[a](./other.md#sub)\n"
	res := Locator{Path: "doc.md"}.Locate([]byte(src), 3, 6)
	assert.Equal(t, TokenFileLink, res.Tag)
	assert.Equal(t, "other.md", res.TargetFile)
	assert.Equal(t, "sub", res.TargetAnchor)
}

func TestLocateRefUse(t *testing.T) {
	t.Parallel()
	src := "# Top\n\nSee [linked][label].\n\n[label]: https://example.com\n"
	res := Locator{Path: "a.md"}.Locate([]byte(src), 3, 8)
	assert.Equal(t, TokenRefUse, res.Tag)
	assert.Equal(t, "label", res.Label)
}

func TestLocateRefDef(t *testing.T) {
	t.Parallel()
	src := "# T\n\n[See][label]\n\n[label]: https://example.com\n"
	res := Locator{Path: "a.md"}.Locate([]byte(src), 5, 3)
	assert.Equal(t, TokenRefDef, res.Tag)
	assert.Equal(t, "label", res.Label)
}

func TestLocateDirectiveArg(t *testing.T) {
	t.Parallel()
	src := strings.Join([]string{
		"# Top",
		"",
		"<?include",
		`file: "x.md"`,
		"?>",
		"<?/include?>",
		"",
	}, "\n")
	res := Locator{Path: "a.md"}.Locate([]byte(src), 4, 8)
	assert.Equal(t, TokenDirectiveArg, res.Tag)
	assert.Equal(t, "include", res.DirectiveName)
	assert.Equal(t, "file", res.DirectiveArg)
	assert.Equal(t, "x.md", res.DirectiveValue)
	assert.Equal(t, "x.md", res.DirectiveTargetFile)
}

func TestLocateFrontMatterKey(t *testing.T) {
	t.Parallel()
	src := "---\ntitle: Hello\nkinds:\n  - guide\n---\n# Body\n"
	res := Locator{Path: "a.md"}.Locate([]byte(src), 2, 2)
	assert.Equal(t, TokenFrontMatterKey, res.Tag)
	assert.Equal(t, "title", res.FrontMatterKey)
}

func TestLocateFrontMatterValue(t *testing.T) {
	t.Parallel()
	src := "---\ntitle: Hello\nkind: guide\n---\n# Body\n"
	// Cursor after the colon on line 3.
	res := Locator{Path: "a.md"}.Locate([]byte(src), 3, 8)
	assert.Equal(t, TokenFrontMatterValue, res.Tag)
	assert.Equal(t, "kind", res.FrontMatterKey)
	assert.Equal(t, "guide", res.FrontMatterValue)
}

func TestLocateFileTop(t *testing.T) {
	t.Parallel()
	src := "# Top\n"
	res := Locator{Path: "a.md"}.Locate([]byte(src), 1, 1)
	assert.Equal(t, TokenFileTop, res.Tag)
}

func TestLocateNoneOnPlainProse(t *testing.T) {
	t.Parallel()
	src := "# Top\n\nordinary text without links\n"
	res := Locator{Path: "a.md"}.Locate([]byte(src), 3, 5)
	assert.Equal(t, TokenNone, res.Tag)
}

func TestLocateFrontMatterKindsListItem(t *testing.T) {
	t.Parallel()
	src := "---\ntitle: T\nkinds:\n  - guide\n  - reference\n---\n# Body\n"
	// Cursor on `  - guide` line (line 4).
	res := Locator{Path: "a.md"}.Locate([]byte(src), 4, 5)
	assert.Equal(t, TokenFrontMatterValue, res.Tag)
	assert.Equal(t, "kinds", res.FrontMatterKey)
	assert.Equal(t, "guide", res.FrontMatterValue)
}

func TestLocateOutOfRangeSafe(t *testing.T) {
	t.Parallel()
	src := "# Top\n"
	res := Locator{Path: "a.md"}.Locate([]byte(src), -1, -1)
	// negative coords clamp to (1, 1) which is FileTop on body line 1.
	assert.Equal(t, TokenFileTop, res.Tag)
	res = Locator{Path: "a.md"}.Locate([]byte(src), 99, 99)
	assert.Equal(t, TokenNone, res.Tag)
}

func TestLocateFrontMatterEmptyKey(t *testing.T) {
	t.Parallel()
	src := "---\nfoo bar baz\n---\n# Body\n"
	res := Locator{Path: "a.md"}.Locate([]byte(src), 2, 2)
	assert.Equal(t, TokenNone, res.Tag)
}

func TestLocateRefDefOnNonRefLine(t *testing.T) {
	t.Parallel()
	src := "# T\n\nplain text\n"
	res := Locator{Path: "a.md"}.Locate([]byte(src), 3, 5)
	assert.Equal(t, TokenNone, res.Tag)
}

func TestLocateBuildDirectiveInputListItem(t *testing.T) {
	t.Parallel()
	// Cursor on an inputs: list item resolves to that input file so
	// go-to-definition works on a <?build?> input.
	src := "# Top\n\n<?build\nrecipe: r\ninputs:\n  - \"x.md\"\noutputs:\n  - out.html\n?>\n<?/build?>\n"
	res := Locator{Path: "a.md"}.Locate([]byte(src), 6, 6)
	assert.Equal(t, TokenDirectiveArg, res.Tag)
	assert.Equal(t, "build", res.DirectiveName)
	assert.Equal(t, "inputs", res.DirectiveArg)
	assert.Equal(t, "x.md", res.DirectiveTargetFile)
}

func TestLocateFileLinkResolvesAgainstSourceDir(t *testing.T) {
	t.Parallel()
	// Source file lives in `docs/`; the relative link `./b.md`
	// resolves to `docs/b.md`, not bare `b.md`.
	src := "# Top\n\n[next](./b.md)\n"
	res := Locator{Path: "docs/a.md"}.Locate([]byte(src), 3, 4)
	assert.Equal(t, TokenFileLink, res.Tag)
	assert.Equal(t, "docs/b.md", res.TargetFile)
}

func TestLocateFileLinkEscapingRoot(t *testing.T) {
	t.Parallel()
	src := "# Top\n\n[bad](../up.md)\n"
	res := Locator{Path: "docs/a.md"}.Locate([]byte(src), 3, 4)
	assert.Equal(t, TokenFileLink, res.Tag)
	// `../up.md` from `docs/a.md` resolves to bare `up.md`.
	assert.Equal(t, "up.md", res.TargetFile)
}

func TestLocateHeadingWithDuplicateAnchorDisambiguates(t *testing.T) {
	t.Parallel()
	src := "# Same\n\n# Same\n\n# Same\n"
	// Cursor on the second `# Same` line (line 3) — slug is
	// disambiguated to "same-1".
	res := Locator{Path: "a.md"}.Locate([]byte(src), 3, 3)
	assert.Equal(t, TokenHeading, res.Tag)
	assert.Equal(t, "same-1", res.Anchor)
}

func TestLocateRefDefOnLineWithoutLabel(t *testing.T) {
	t.Parallel()
	src := "# T\n\n# Other heading\n"
	res := Locator{Path: "a.md"}.Locate([]byte(src), 3, 3)
	// Heading on this line, not a ref def.
	assert.Equal(t, TokenHeading, res.Tag)
}

func TestLocateCursorAfterLinkOnSameLine(t *testing.T) {
	t.Parallel()
	// Cursor sits on the prose after a link on the same line. The
	// previous bound stretched the link's range to end-of-line and
	// would mis-tag this position as TokenAnchorLink.
	src := "# T\n\nSee [a](#sec) and then plain prose here.\n\n## Sec\n"
	// Column 28 is somewhere in `plain prose here`.
	res := Locator{Path: "a.md"}.Locate([]byte(src), 3, 28)
	assert.NotEqual(t, TokenAnchorLink, res.Tag,
		"cursor in trailing prose must not be tagged as link, got %+v", res)
}

func TestLocateRefStyleInlineLink(t *testing.T) {
	t.Parallel()
	// Reference-style link: cursor inside `[text][label]` should
	// surface as TokenRefUse; cursor on the prose after the link
	// must not.
	src := "# T\n\nSee [text][lab] here.\n\n[lab]: https://x.com\n"
	res := Locator{Path: "a.md"}.Locate([]byte(src), 3, 8)
	assert.Equal(t, TokenRefUse, res.Tag)
	res = Locator{Path: "a.md"}.Locate([]byte(src), 3, 22)
	assert.NotEqual(t, TokenRefUse, res.Tag)
}

func TestLocateEmptyAnchorOnRefDef(t *testing.T) {
	t.Parallel()
	src := "# T\n\n[label]: https://x.com\n"
	res := Locator{Path: "a.md"}.Locate([]byte(src), 3, 3)
	assert.Equal(t, TokenRefDef, res.Tag)
	assert.Equal(t, "label", res.Label)
}

func TestLocatePIContainsLineMultiline(t *testing.T) {
	t.Parallel()
	src := "# T\n\n<?include\nfile: \"x.md\"\nstrip-frontmatter: \"true\"\n?>\n<?/include?>\n"
	// Line 5 is inside the multi-line PI.
	res := Locator{Path: "a.md"}.Locate([]byte(src), 5, 1)
	assert.Equal(t, TokenDirectiveArg, res.Tag)
	assert.Equal(t, "include", res.DirectiveName)
}

func TestLocatePIWithSingleLineClosure(t *testing.T) {
	t.Parallel()
	src := "# T\n\n<?allow-empty-section?>\n"
	res := Locator{Path: "a.md"}.Locate([]byte(src), 3, 5)
	assert.Equal(t, TokenDirectiveArg, res.Tag)
	assert.Equal(t, "allow-empty-section", res.DirectiveName)
}

func TestSafeURLPathEscape(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "abc", SafeURLPathEscape("abc"))
	assert.Contains(t, SafeURLPathEscape("a b"), "%20")
}

func TestLocateFrontMatterKindsListItemWithDifferentValues(t *testing.T) {
	t.Parallel()
	src := "---\nkinds:\n  - guide\n  - reference\n---\n# Body\n"
	// Cursor on the second list item.
	res := Locator{Path: "a.md"}.Locate([]byte(src), 4, 5)
	assert.Equal(t, TokenFrontMatterValue, res.Tag)
	assert.Equal(t, "kinds", res.FrontMatterKey)
	assert.Equal(t, "reference", res.FrontMatterValue)
}

func TestEnclosingListKey_FindsParentKey(t *testing.T) {
	lines := [][]byte{
		[]byte("inputs:"),
		[]byte("  - alpha"),
		[]byte("  - beta"),
		[]byte("  - gamma"),
	}
	// Line 4 (1-based) is "gamma"; the enclosing key is "inputs".
	got := enclosingListKey(lines, 4)
	assert.Equal(t, "inputs", got)
}

// --- Dedicated tests for unexported helpers in locate.go ---

// parseDoc parses body-only markdown (no front matter) and returns root + source.
func parseDoc(src string) (goldast.Node, []byte) {
	b := []byte(src)
	root := lint.NewParser().Parse(text.NewReader(b), parser.WithContext(parser.NewContext()))
	return root, b
}

// firstLink returns the first *ast.Link in root, or nil.
func firstLink(root goldast.Node) *goldast.Link {
	var found *goldast.Link
	_ = goldast.Walk(root, func(n goldast.Node, entering bool) (goldast.WalkStatus, error) {
		if !entering {
			return goldast.WalkContinue, nil
		}
		if l, ok := n.(*goldast.Link); ok {
			found = l
			return goldast.WalkStop, nil
		}
		return goldast.WalkContinue, nil
	})
	return found
}

// firstPI returns the first *piparser.ProcessingInstruction in root, or nil.
func firstPI(root goldast.Node) *piparser.ProcessingInstruction {
	var found *piparser.ProcessingInstruction
	_ = goldast.Walk(root, func(n goldast.Node, entering bool) (goldast.WalkStatus, error) {
		if !entering {
			return goldast.WalkContinue, nil
		}
		if pi, ok := n.(*piparser.ProcessingInstruction); ok {
			found = pi
			return goldast.WalkStop, nil
		}
		return goldast.WalkContinue, nil
	})
	return found
}

// nthHeading returns the n-th (1-based) heading in root, or nil.
func nthHeading(root goldast.Node, n int) *goldast.Heading {
	count := 0
	var found *goldast.Heading
	_ = goldast.Walk(root, func(node goldast.Node, entering bool) (goldast.WalkStatus, error) {
		if !entering {
			return goldast.WalkContinue, nil
		}
		h, ok := node.(*goldast.Heading)
		if !ok {
			return goldast.WalkContinue, nil
		}
		count++
		if count == n {
			found = h
			return goldast.WalkStop, nil
		}
		return goldast.WalkContinue, nil
	})
	return found
}

func TestHeadingInfo(t *testing.T) {
	t.Parallel()
	src := "# Alpha\n\n# Alpha\n\n## Beta\n"
	root, b := parseDoc(src)

	h1 := nthHeading(root, 1)
	h2 := nthHeading(root, 2)
	h3 := nthHeading(root, 3)
	require.NotNil(t, h1)
	require.NotNil(t, h2)
	require.NotNil(t, h3)

	anchor, level, name := headingInfo(h1, b, root)
	assert.Equal(t, "alpha", anchor)
	assert.Equal(t, 1, level)
	assert.Equal(t, "Alpha", name)

	// Duplicate slug disambiguated with suffix.
	anchor2, _, _ := headingInfo(h2, b, root)
	assert.Equal(t, "alpha-1", anchor2)

	anchor3, level3, name3 := headingInfo(h3, b, root)
	assert.Equal(t, "beta", anchor3)
	assert.Equal(t, 2, level3)
	assert.Equal(t, "Beta", name3)
}

func TestLocateInAST(t *testing.T) {
	t.Parallel()
	src := "# T\n\n[link](./a.md)\n"
	root, b := parseDoc(src)
	lines := bytes.Split(b, []byte("\n"))

	// Line 3, col 3 is inside the link text "link".
	res, ok := locateInAST("doc.md", root, b, lines, 3, 3)
	assert.True(t, ok)
	assert.Equal(t, TokenFileLink, res.Tag)
	assert.Equal(t, "a.md", res.TargetFile)

	// Line 1, col 1 is on the heading — no link or PI.
	_, ok = locateInAST("doc.md", root, b, lines, 1, 1)
	assert.False(t, ok)
}

func TestLinkContainsOffset(t *testing.T) {
	t.Parallel()
	// "# T\n\n" = 5 bytes; "[text](./a.md)" starts at offset 5.
	src := "# T\n\n[text](./a.md)\n"
	root, b := parseDoc(src)
	l := firstLink(root)
	require.NotNil(t, l)

	// Offset 7 is 'e' in "text" — inside the link.
	assert.True(t, linkContainsOffset(b, l, 7))
	// Offset 0 is '#' — before the link.
	assert.False(t, linkContainsOffset(b, l, 0))
}

func TestLinkCloseOffset(t *testing.T) {
	t.Parallel()
	// "[text](dest)\n": '[' 0, text 1-4, ']' 5, '(' 6, dest 7-10, ')' 11, '\n' 12.
	// With nil link (inline path): after=5, source[5]=']' so i advances to 6,
	// source[6]='(' triggers depth scan, ')' found at offset 11.
	src := []byte("[text](dest)\n")
	assert.Equal(t, 11, linkCloseOffset(src, nil, 5))

	// Newline before the closing ')' → -1.
	srcBroken := []byte("[text](dest\nmore\n")
	assert.Equal(t, -1, linkCloseOffset(srcBroken, nil, 5))
}

func TestScanForByte(t *testing.T) {
	t.Parallel()
	src := []byte("ab]cd")
	assert.Equal(t, 2, scanForByte(src, 0, ']'))
	// Start past the target.
	assert.Equal(t, -1, scanForByte(src, 3, ']'))
	// Newline stops the scan before the target.
	src2 := []byte("ab\n]")
	assert.Equal(t, -1, scanForByte(src2, 0, ']'))
	// Target not present at all.
	assert.Equal(t, -1, scanForByte(src, 0, 'z'))
}

func TestLinkToLocate(t *testing.T) {
	t.Parallel()

	// Inline file link.
	root, b := parseDoc("# T\n\n[text](./a.md)\n")
	l := firstLink(root)
	require.NotNil(t, l)
	res := linkToLocate("doc.md", l, b)
	assert.Equal(t, TokenFileLink, res.Tag)
	assert.Equal(t, "a.md", res.TargetFile)

	// Reference-use link.
	root2, b2 := parseDoc("# T\n\n[text][label]\n\n[label]: https://x.com\n")
	l2 := firstLink(root2)
	require.NotNil(t, l2)
	res2 := linkToLocate("doc.md", l2, b2)
	assert.Equal(t, TokenRefUse, res2.Tag)
	assert.Equal(t, "label", res2.Label)

	// Anchor-only link.
	root3, b3 := parseDoc("# T\n\n[here](#sec)\n")
	l3 := firstLink(root3)
	require.NotNil(t, l3)
	res3 := linkToLocate("doc.md", l3, b3)
	assert.Equal(t, TokenAnchorLink, res3.Tag)
	assert.Equal(t, "sec", res3.TargetAnchor)
}

func TestPiToLocate(t *testing.T) {
	t.Parallel()
	src := "# T\n\n<?include\nfile: \"x.md\"\n?>\n<?/include?>\n"
	root, b := parseDoc(src)
	pi := firstPI(root)
	require.NotNil(t, pi)

	lines := bytes.Split(b, []byte("\n"))
	// Line 4 is `file: "x.md"`.
	res := piToLocate(pi, b, lines, 4, 8)
	assert.Equal(t, TokenDirectiveArg, res.Tag)
	assert.Equal(t, "include", res.DirectiveName)
	assert.Equal(t, "file", res.DirectiveArg)
	assert.Equal(t, "x.md", res.DirectiveValue)
	assert.Equal(t, "x.md", res.DirectiveTargetFile)
}

func TestListItemValue(t *testing.T) {
	t.Parallel()
	v, ok := listItemValue("  - foo")
	assert.True(t, ok)
	assert.Equal(t, "foo", v)

	v, ok = listItemValue(`  - "bar"`)
	assert.True(t, ok)
	assert.Equal(t, "bar", v)

	_, ok = listItemValue("not a list item")
	assert.False(t, ok)

	_, ok = listItemValue("key: value")
	assert.False(t, ok)
}

func TestHeadingOnLine(t *testing.T) {
	t.Parallel()
	src := "# Top\n\nSome text\n\n## Sub\n"
	root, b := parseDoc(src)

	h := headingOnLine(root, b, 1)
	require.NotNil(t, h)
	assert.Equal(t, 1, h.Level)

	h = headingOnLine(root, b, 3)
	assert.Nil(t, h)

	h = headingOnLine(root, b, 5)
	require.NotNil(t, h)
	assert.Equal(t, 2, h.Level)
}

func TestFrontMatterListItem(t *testing.T) {
	t.Parallel()
	v, ok := frontMatterListItem("- foo")
	assert.True(t, ok)
	assert.Equal(t, "foo", v)

	v, ok = frontMatterListItem("  - bar")
	assert.True(t, ok)
	assert.Equal(t, "bar", v)

	v, ok = frontMatterListItem("-")
	assert.True(t, ok)
	assert.Equal(t, "", v)

	_, ok = frontMatterListItem("key: val")
	assert.False(t, ok)
}

func TestFrontMatterParentKey(t *testing.T) {
	t.Parallel()
	lines := [][]byte{
		[]byte("kinds:"),
		[]byte("  - guide"),
		[]byte("  - reference"),
	}
	assert.Equal(t, "kinds", frontMatterParentKey(lines, 2))
	assert.Equal(t, "kinds", frontMatterParentKey(lines, 1))
	assert.Equal(t, "", frontMatterParentKey(lines, 0))
}

func TestOffsetAt(t *testing.T) {
	t.Parallel()
	lines := [][]byte{
		[]byte("abc"),
		[]byte("de"),
		[]byte("f"),
	}
	// (1,1) → 0
	assert.Equal(t, 0, offsetAt(lines, 1, 1))
	// (1,2) → 1
	assert.Equal(t, 1, offsetAt(lines, 1, 2))
	// (2,1) → len("abc")+newline = 4
	assert.Equal(t, 4, offsetAt(lines, 2, 1))
	// (2,2) → 5
	assert.Equal(t, 5, offsetAt(lines, 2, 2))
	// Clamp: line < 1 → treated as line 1
	assert.Equal(t, 0, offsetAt(lines, 0, 1))
}
