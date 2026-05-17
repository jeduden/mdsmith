package index

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
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

func TestCompletionContextLineZero(t *testing.T) {
	t.Parallel()
	// line=0 is clamped to 1; line 1 of body is "# Heading" → no anchor context.
	src := "# Heading\n\nSee [link](#\n"
	res := Locator{Path: "a.md"}.CompletionContext([]byte(src), 0, 12)
	assert.Equal(t, CompletionNone, res.Tag)
}

func TestCompletionContextColZero(t *testing.T) {
	t.Parallel()
	// col=0 is clamped to 1; at col=1 the text-before-cursor is empty → no match.
	src := "# Top\n\nSee [link](#heading\n"
	res := Locator{Path: "a.md"}.CompletionContext([]byte(src), 3, 0)
	assert.Equal(t, CompletionNone, res.Tag)
}

func TestCompletionContextLineOutOfRange(t *testing.T) {
	t.Parallel()
	// line=999 is beyond the document → bodyLine > len(bodyLines) → CompletionNone.
	src := "# Heading\n\nShort doc.\n"
	res := Locator{Path: "a.md"}.CompletionContext([]byte(src), 999, 1)
	assert.Equal(t, CompletionNone, res.Tag)
}

func TestCompletionContextDirectiveColBeyondLine(t *testing.T) {
	t.Parallel()
	// col=100 on a short PI line triggers the cursorByteCol > len(lineBytes) clamp.
	src := strings.Join([]string{
		"# Top",
		"",
		"<?catalog",
		`glob: "docs/"`,
		"?>",
		"<?/catalog?>",
		"",
	}, "\n")
	// Line 4 is `glob: "docs/"` (13 chars). col=100 is clamped to len.
	res := Locator{Path: "a.md"}.CompletionContext([]byte(src), 4, 100)
	assert.Equal(t, CompletionDirectivePath, res.Tag)
	assert.Equal(t, "docs/", res.Prefix)
}

func TestCompletionContextScanBackwardListItemContinue(t *testing.T) {
	t.Parallel()
	// Another list item above the cursor triggers the "skip list item" continue
	// (L250-251) in scanBackwardForPIKey before reaching the parent "glob:" key.
	src := strings.Join([]string{
		"# Top",
		"",
		"<?catalog",
		"glob:",
		`  - "docs/*.md"`,
		`  - "docs/g`,
		"?>",
		"<?/catalog?>",
		"",
	}, "\n")
	// Line 6, cursor at end of `  - "docs/g`.
	res := Locator{Path: "a.md"}.CompletionContext([]byte(src), 6, 12)
	assert.Equal(t, CompletionDirectivePath, res.Tag)
	assert.Equal(t, "docs/g", res.Prefix)
	assert.Equal(t, "glob", res.DirectiveArg)
}

func TestCompletionContextScanBackwardYAMLComment(t *testing.T) {
	t.Parallel()
	// A YAML comment line (# ...) between glob: and a list item is skipped so
	// catalog glob completion still fires (scanBackwardForPIKey continues past it).
	src := strings.Join([]string{
		"# Top",
		"",
		"<?catalog",
		"glob:",
		"  # valid YAML comment",
		`  - "docs/g`,
		"?>",
		"<?/catalog?>",
		"",
	}, "\n")
	res := Locator{Path: "a.md"}.CompletionContext([]byte(src), 6, 12)
	assert.Equal(t, CompletionDirectivePath, res.Tag)
	assert.Equal(t, "docs/g", res.Prefix)
	assert.Equal(t, "glob", res.DirectiveArg)
}

func TestCompletionContextScanBackwardBreak(t *testing.T) {
	t.Parallel()
	// A non-key, non-list, non-comment line with no colon causes the scanner
	// to break before finding the parent key → CompletionNone.
	src := strings.Join([]string{
		"# Top",
		"",
		"<?catalog",
		"glob:",
		"  word without colon",
		`  - "docs/g`,
		"?>",
		"<?/catalog?>",
		"",
	}, "\n")
	res := Locator{Path: "a.md"}.CompletionContext([]byte(src), 6, 12)
	assert.Equal(t, CompletionNone, res.Tag)
}

func TestScanBackwardForPIKeyExhaustsLoop(t *testing.T) {
	t.Parallel()
	// All preceding lines are list items; the for-loop exhausts and returns ""
	// (L258) — verifies the trailing return after the loop.
	lines := [][]byte{
		[]byte("  - alpha"),
		[]byte("  - beta"),
		[]byte(`  - "docs/g`),
	}
	result := scanBackwardForPIKey(lines, 2)
	assert.Equal(t, "", result)
}

func TestDirectiveCompletionContextLineGuard(t *testing.T) {
	t.Parallel()
	// line=0 is out of bounds → returns CompletionNone (L161-163).
	pi := &lint.ProcessingInstruction{Name: "catalog"}
	lines := [][]byte{[]byte(`glob: "docs/"`)}
	res := directiveCompletionContext(pi, lines, 0, 5)
	assert.Equal(t, CompletionNone, res.Tag)
}

func TestDirectiveCompletionContextColNegative(t *testing.T) {
	t.Parallel()
	// col=0 makes cursorByteCol = -1 < 0 → clamped to 0 (L167-169).
	// With empty text-up-to-cursor, piArgRE and yamlListItemValue both fail → CompletionNone.
	pi := &lint.ProcessingInstruction{Name: "catalog"}
	lines := [][]byte{[]byte(`glob: "docs/"`)}
	res := directiveCompletionContext(pi, lines, 1, 0)
	assert.Equal(t, CompletionNone, res.Tag)
}

func TestCodeNodeContainsLineEmpty(t *testing.T) {
	t.Parallel()
	// A code block node with no content lines (Len() == 0) → returns false (L285-287).
	body := []byte("# Top\n\n```\n```\n")
	block := ast.NewFencedCodeBlock(nil)
	assert.False(t, codeNodeContainsLine(body, block, 3))
}

func TestCompletionContextAnchorOtherFileUppercaseExt(t *testing.T) {
	t.Parallel()
	// Cross-file anchor completion triggers even when the extension is uppercase
	// (e.g. .MD). compAnchorOtherFileRE uses (?i:md|markdown) for case-insensitive
	// extension matching consistent with isMarkdownExt.
	src := "# Top\n\nSee [link](./OTHER.MD#sec\n"
	res := Locator{Path: "a.md"}.CompletionContext([]byte(src), 3, 26)
	assert.Equal(t, CompletionAnchorOtherFile, res.Tag)
	assert.Equal(t, "sec", res.Prefix)
}

func TestCompletionContextDirectiveSingleLine(t *testing.T) {
	t.Parallel()
	// Single-line PI form: all args on the same line as the opener.
	// piArgRE is anchored at ^ so a plain FindStringSubmatch on the full line
	// fails; directiveCompletionContext must strip the "<?include " opener
	// before retrying the regex.
	src := "# Top\n\n<?include file: \"docs/guide.md\"?>\n"
	// "<?include file: \"docs/g" is 23 bytes → col 24 places cursor after 'g'.
	res := Locator{Path: "a.md"}.CompletionContext([]byte(src), 3, 24)
	assert.Equal(t, CompletionDirectivePath, res.Tag)
	assert.Equal(t, "docs/g", res.Prefix)
	assert.Equal(t, "include", res.DirectiveName)
	assert.Equal(t, "file", res.DirectiveArg)
}

func TestCodeNodeContainsLineNoSpillover(t *testing.T) {
	t.Parallel()
	// Goldmark segments include the trailing newline in Stop. Without the
	// stop-- fix, codeNodeContainsLine would report that the line immediately
	// after a code block is inside it, suppressing completion there.
	// Build a source with an indented code block followed by plain text:
	//   line 1: # Top
	//   line 2: (empty)
	//   line 3:     indented code
	//   line 4: (empty — ends indented code block)
	//   line 5: normal text
	body := []byte("# Top\n\n    indented code\n\nnormal text\n")
	root := lint.NewParser().Parse(text.NewReader(body), parser.WithContext(parser.NewContext()))
	// Line 3 (the code line) must be inside the block.
	assert.True(t, insideCodeBlock(root, body, 3), "line 3 should be inside code block")
	// Line 5 (normal text after the blank separator) must NOT be inside the block.
	assert.False(t, insideCodeBlock(root, body, 5), "line 5 must not be inside code block")
}
