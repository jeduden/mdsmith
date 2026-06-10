package index

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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
