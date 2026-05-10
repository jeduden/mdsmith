package index

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompletionContextAnchorCurrentFile(t *testing.T) {
	t.Parallel()
	src := "# Heading One\n\n## Section Two\n\nSee [here](#he\n"
	// Cursor after "he" on line 5, column 15 (after the '#').
	res := Locator{Path: "a.md"}.CompletionContext([]byte(src), 5, 15)
	assert.Equal(t, CompletionAnchorCurrentFile, res.Tag)
	assert.Equal(t, "he", res.Prefix)
}

func TestCompletionContextAnchorCurrentFileEmpty(t *testing.T) {
	t.Parallel()
	src := "# Top\n\nSee [link](#\n"
	// Cursor right after '#' (col 13 = after 12-char "See [link](#").
	res := Locator{Path: "a.md"}.CompletionContext([]byte(src), 3, 13)
	assert.Equal(t, CompletionAnchorCurrentFile, res.Tag)
	assert.Equal(t, "", res.Prefix)
}

func TestCompletionContextAnchorOtherFile(t *testing.T) {
	t.Parallel()
	src := "# Top\n\nSee [link](./other.md#sec\n"
	// "See [link](./other.md#sec" = 25 chars; cursor after last 'c' = col 26.
	res := Locator{Path: "a.md"}.CompletionContext([]byte(src), 3, 26)
	assert.Equal(t, CompletionAnchorOtherFile, res.Tag)
	assert.Equal(t, "sec", res.Prefix)
	assert.Equal(t, "other.md", res.TargetFile)
}

func TestCompletionContextAnchorOtherFileSubdir(t *testing.T) {
	t.Parallel()
	src := "# Top\n\nSee [link](./sub/doc.md#\n"
	// "See [link](./sub/doc.md#" = 24 chars; cursor after '#' = col 25.
	res := Locator{Path: "docs/a.md"}.CompletionContext([]byte(src), 3, 25)
	assert.Equal(t, CompletionAnchorOtherFile, res.Tag)
	assert.Equal(t, "", res.Prefix)
	assert.Equal(t, "docs/sub/doc.md", res.TargetFile)
}

func TestCompletionContextRefLabel(t *testing.T) {
	t.Parallel()
	src := "# Top\n\nSee [linked][la\n\n[label]: https://example.com\n"
	// Cursor after "la" on line 3.
	res := Locator{Path: "a.md"}.CompletionContext([]byte(src), 3, 16)
	assert.Equal(t, CompletionRefLabel, res.Tag)
	assert.Equal(t, "la", res.Prefix)
}

func TestCompletionContextRefLabelEmpty(t *testing.T) {
	t.Parallel()
	src := "# Top\n\nSee [text][\n\n[foo]: https://example.com\n"
	// "See [text][" = 11 chars; cursor after '[' = col 12.
	res := Locator{Path: "a.md"}.CompletionContext([]byte(src), 3, 12)
	assert.Equal(t, CompletionRefLabel, res.Tag)
	assert.Equal(t, "", res.Prefix)
}

func TestCompletionContextKindScalar(t *testing.T) {
	t.Parallel()
	src := "---\nkind: gui\n---\n# Body\n"
	// Cursor on "gui" value (line 2, col 8).
	res := Locator{Path: "a.md"}.CompletionContext([]byte(src), 2, 8)
	assert.Equal(t, CompletionKindValue, res.Tag)
	assert.Equal(t, "gui", res.Prefix)
	assert.Equal(t, "kind", res.FrontMatterKey)
}

func TestCompletionContextKindListItem(t *testing.T) {
	t.Parallel()
	src := "---\nkinds:\n  - gui\n---\n# Body\n"
	// Cursor on list item "gui" (line 3, col 7).
	res := Locator{Path: "a.md"}.CompletionContext([]byte(src), 3, 7)
	assert.Equal(t, CompletionKindValue, res.Tag)
	assert.Equal(t, "gui", res.Prefix)
	assert.Equal(t, "kinds", res.FrontMatterKey)
}

func TestCompletionContextKindListItemEmpty(t *testing.T) {
	t.Parallel()
	src := "---\nkinds:\n  - \n---\n# Body\n"
	// Cursor after "- " - empty prefix (line 3)
	res := Locator{Path: "a.md"}.CompletionContext([]byte(src), 3, 5)
	assert.Equal(t, CompletionKindValue, res.Tag)
	assert.Equal(t, "", res.Prefix)
	assert.Equal(t, "kinds", res.FrontMatterKey)
}

func TestCompletionContextDirectiveInclude(t *testing.T) {
	t.Parallel()
	src := strings.Join([]string{
		"# Top",
		"",
		"<?include",
		`file: "docs/g"`,
		"?>",
		"<?/include?>",
		"",
	}, "\n")
	// Cursor on "docs/g" value (line 4, col 14).
	res := Locator{Path: "a.md"}.CompletionContext([]byte(src), 4, 14)
	assert.Equal(t, CompletionDirectivePath, res.Tag)
	assert.Equal(t, "docs/g", res.Prefix)
	assert.Equal(t, "include", res.DirectiveName)
	assert.Equal(t, "file", res.DirectiveArg)
}

func TestCompletionContextDirectiveBuild(t *testing.T) {
	t.Parallel()
	src := strings.Join([]string{
		"# Top",
		"",
		"<?build",
		`source: "internal/`,
		"?>",
		"",
	}, "\n")
	// Cursor after "internal/" (line 4, col 19).
	res := Locator{Path: "a.md"}.CompletionContext([]byte(src), 4, 19)
	assert.Equal(t, CompletionDirectivePath, res.Tag)
	assert.Equal(t, "internal/", res.Prefix)
	assert.Equal(t, "build", res.DirectiveName)
	assert.Equal(t, "source", res.DirectiveArg)
}

func TestCompletionContextDirectiveCatalogInline(t *testing.T) {
	t.Parallel()
	src := strings.Join([]string{
		"# Top",
		"",
		"<?catalog",
		`glob: "docs/`,
		"?>",
		"<?/catalog?>",
		"",
	}, "\n")
	// Cursor after "docs/" (line 4, col 13).
	res := Locator{Path: "a.md"}.CompletionContext([]byte(src), 4, 13)
	assert.Equal(t, CompletionDirectivePath, res.Tag)
	assert.Equal(t, "docs/", res.Prefix)
	assert.Equal(t, "catalog", res.DirectiveName)
	assert.Equal(t, "glob", res.DirectiveArg)
}

func TestCompletionContextDirectiveCatalogListItem(t *testing.T) {
	t.Parallel()
	src := strings.Join([]string{
		"# Top",
		"",
		"<?catalog",
		"glob:",
		`  - "docs/`,
		"?>",
		"<?/catalog?>",
		"",
	}, "\n")
	// Cursor after "docs/" in list item (line 5, col 11).
	res := Locator{Path: "a.md"}.CompletionContext([]byte(src), 5, 11)
	assert.Equal(t, CompletionDirectivePath, res.Tag)
	assert.Equal(t, "docs/", res.Prefix)
	assert.Equal(t, "catalog", res.DirectiveName)
	assert.Equal(t, "glob", res.DirectiveArg)
}

func TestCompletionContextNoneOnPlainText(t *testing.T) {
	t.Parallel()
	src := "# Heading\n\nJust plain prose here.\n"
	res := Locator{Path: "a.md"}.CompletionContext([]byte(src), 3, 10)
	assert.Equal(t, CompletionNone, res.Tag)
}

func TestCompletionContextNoneInsideFencedCodeBlock(t *testing.T) {
	t.Parallel()
	src := "# Top\n\n```\n[link](#anchor\n```\n"
	// Cursor inside the code block on line 4.
	res := Locator{Path: "a.md"}.CompletionContext([]byte(src), 4, 10)
	assert.Equal(t, CompletionNone, res.Tag)
}

func TestCompletionContextNoneForImageLink(t *testing.T) {
	t.Parallel()
	src := "# Top\n\n![alt](#\n"
	// Image links should NOT trigger anchor completion.
	res := Locator{Path: "a.md"}.CompletionContext([]byte(src), 3, 8)
	assert.Equal(t, CompletionNone, res.Tag)
}

func TestCompletionContextNoneForNonKindFrontMatter(t *testing.T) {
	t.Parallel()
	src := "---\ntitle: foo\n---\n# Body\n"
	// Cursor on "title" value should not trigger kind completion.
	res := Locator{Path: "a.md"}.CompletionContext([]byte(src), 2, 8)
	assert.Equal(t, CompletionNone, res.Tag)
}

func TestCompletionContextNoneFMKeyPosition(t *testing.T) {
	t.Parallel()
	// Cursor on the key side of a FM entry (col ≤ colon) → TokenFrontMatterKey,
	// not TokenFrontMatterValue → condition at completionContextFrontMatter is false.
	src := "---\ntitle: foo\n---\n# Body\n"
	res := Locator{Path: "a.md"}.CompletionContext([]byte(src), 2, 3)
	assert.Equal(t, CompletionNone, res.Tag)
}

func TestCompletionContextNoneInsideIndentedCodeBlock(t *testing.T) {
	t.Parallel()
	// 4-space indented code block suppresses completion (*ast.CodeBlock).
	src := "# Top\n\nPara.\n\n    [link](#anchor\n    more code\n\nEnd.\n"
	res := Locator{Path: "a.md"}.CompletionContext([]byte(src), 5, 12)
	assert.Equal(t, CompletionNone, res.Tag)
}

func TestCompletionContextCatalogBareListItem(t *testing.T) {
	t.Parallel()
	// Bare "-" list item (no trailing space) → empty prefix. A blank line
	// between "glob:" and "-" exercises the empty-line skip in scanBackwardForPIKey.
	src := strings.Join([]string{
		"# Top",
		"",
		"<?catalog",
		"glob:",
		"",
		"  -",
		"?>",
		"<?/catalog?>",
		"",
	}, "\n")
	// Cursor at end of "  -" (line 6, col 4).
	res := Locator{Path: "a.md"}.CompletionContext([]byte(src), 6, 4)
	assert.Equal(t, CompletionDirectivePath, res.Tag)
	assert.Equal(t, "", res.Prefix)
	assert.Equal(t, "glob", res.DirectiveArg)
}

func TestCompletionContextNoneCatalogNonListLine(t *testing.T) {
	t.Parallel()
	// Cursor on "sort: path" (non-list line) inside catalog PI → yamlListItemValue
	// returns false → CompletionNone.
	src := strings.Join([]string{
		"# Top",
		"",
		"<?catalog",
		"glob:",
		`  - "docs/*.md"`,
		"sort: path",
		"?>",
		"<?/catalog?>",
		"",
	}, "\n")
	// Cursor on "sort: path" line (line 6), col 7.
	res := Locator{Path: "a.md"}.CompletionContext([]byte(src), 6, 7)
	assert.Equal(t, CompletionNone, res.Tag)
}

func TestCompletionContextNoneCatalogListItemNonGlobKey(t *testing.T) {
	t.Parallel()
	// List item under a non-glob key → parentKey != "glob" → CompletionNone.
	src := strings.Join([]string{
		"# Top",
		"",
		"<?catalog",
		"glob:",
		`  - "docs/*.md"`,
		"fields:",
		"  - title",
		"?>",
		"<?/catalog?>",
		"",
	}, "\n")
	// Cursor on "  - title" (line 7), col 9.
	res := Locator{Path: "a.md"}.CompletionContext([]byte(src), 7, 9)
	assert.Equal(t, CompletionNone, res.Tag)
}

func TestCompletionContextAnchorOtherFileEscapingWorkspace(t *testing.T) {
	t.Parallel()
	src := "# Top\n\nSee [link](../../escape.md#sec\n"
	// Path escaping workspace — TargetFile should be empty.
	res := Locator{Path: "a.md"}.CompletionContext([]byte(src), 3, 36)
	// Either CompletionNone or CompletionAnchorOtherFile with empty TargetFile.
	if res.Tag == CompletionAnchorOtherFile {
		assert.Equal(t, "", res.TargetFile)
	} else {
		assert.Equal(t, CompletionNone, res.Tag)
	}
}
