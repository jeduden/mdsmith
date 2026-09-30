package refactor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMove_OutboundEscapedDestinationsLeftAsWritten pins that the moved
// file's own destination spelled with an entity or a backslash escape
// is left as written: a renderer reads `a&amp;b.md` as `a&b.md` and
// `a\_b.md` as `a_b.md`, which the move does not decode, so a rewrite
// would name another file. A Windows-style `sub\c.md`, whose `\` does
// not escape a punctuation byte, is still recomputed.
func TestMove_OutboundEscapedDestinationsLeftAsWritten(t *testing.T) {
	a := "[x](a&amp;b.md) [y](a\\_b.md) [z](a\\).md) [w](sub\\c.md)\n"
	got := moveAndApply(t, map[string]string{
		"a.md":     a,
		"a&b.md":   "# X\n",
		"a_b.md":   "# Y\n",
		"a).md":    "# Z\n",
		"sub/c.md": "# W\n",
	}, "a.md", "docs/a.md")
	assert.Equal(t, "[x](a&amp;b.md) [y](a\\_b.md) [z](a\\).md) [w](../sub/c.md)\n", got["a.md"])
}

// TestMove_IncomingEscapedDestinationsLeftAsWritten pins the incoming
// side: `a\_b.md` reads as `a_b.md`, not as the moved `a/_b.md`, so it
// is left alone, like an entity that spells the path. An escape or an
// entity in the fragment does not touch the path, which is repointed.
func TestMove_IncomingEscapedDestinationsLeftAsWritten(t *testing.T) {
	b := "[y](a\\_b.md) [e](a/&#95;b.md) [v](a/_b.md) [f](a/_b.md#s\\_1&amp;2)\n"
	got := moveAndApply(t, map[string]string{
		"a/_b.md": "# A\n",
		"b.md":    b,
	}, "a/_b.md", "c/_b.md")
	assert.Equal(t, "[y](a\\_b.md) [e](a/&#95;b.md) [v](c/_b.md) [f](c/_b.md#s\\_1&amp;2)\n", got["b.md"])
}

// TestMove_ColonInFirstSegmentGetsDotSlash pins that a new path whose
// first segment holds a `:` is written with a `./` prefix: a bare
// `a:b.md` reads as the URL scheme `a:`, so it would name no workspace
// file. The bare, angle and ref-def forms and the moved file's link to
// itself all get the prefix; a `:` in a later segment needs none.
func TestMove_ColonInFirstSegmentGetsDotSlash(t *testing.T) {
	got := moveAndApply(t, map[string]string{
		"x.md":   "[s](x.md) [e](e.md)\n",
		"e.md":   "# E\n",
		"d/c.md": "[x](../x.md) [y](<../x.md>)\n\n[r]: ../x.md\n",
	}, "x.md", "d/a:b.md")
	assert.Equal(t, "[x](./a:b.md) [y](<./a:b.md>)\n\n[r]: ./a:b.md\n", got["d/c.md"])
	assert.Equal(t, "[s](./a:b.md) [e](../e.md)\n", got["x.md"])

	got = moveAndApply(t, map[string]string{
		"x.md": "# X\n",
		"c.md": "[x](x.md) [y](./x.md)\n",
	}, "x.md", "n:1/a.md")
	assert.Equal(t, "[x](./n:1/a.md) [y](./n:1/a.md)\n", got["c.md"])

	got = moveAndApply(t, map[string]string{
		"x.md": "# X\n",
		"c.md": "[x](x.md)\n",
	}, "x.md", "d/n:1.md")
	assert.Equal(t, "[x](d/n:1.md)\n", got["c.md"])
}

// TestMove_TrailingSlashKeptOnDirectoryLinks pins that the moved file's
// link to a directory keeps its trailing `/` when it is recomputed,
// with or without a fragment, while a trailing `/` after a file name is
// dropped. An incoming `[x](a.md/)` becomes `docs/a.md` for the same
// reason: the `/` after a file name is a typo the rewrite fixes.
func TestMove_TrailingSlashKeptOnDirectoryLinks(t *testing.T) {
	got := moveAndApply(t, map[string]string{
		"a.md":      "[d](sub/) [f](sub/#x) [h](./) [g](./docs/) [b](b.md/)\n",
		"b.md":      "# B\n",
		"c.md":      "[x](a.md/)\n",
		"sub/e.md":  "# E\n",
		"docs/z.md": "# Z\n",
	}, "a.md", "docs/a.md")
	assert.Equal(t, "[d](../sub/) [f](../sub/#x) [h](../) [g](./) [b](../b.md)\n", got["a.md"])
	assert.Equal(t, "[x](docs/a.md)\n", got["c.md"])
}

// TestMove_OutboundAboveRootLeftAsWritten pins the limit move.md lists
// under "What needs a manual fix": the moved file's link that climbs
// out of the workspace names no workspace file, so it is never
// recomputed, though from the new directory it names README.md.
func TestMove_OutboundAboveRootLeftAsWritten(t *testing.T) {
	got := moveAndApply(t, map[string]string{
		"a.md":      "[p](../README.md)\n",
		"README.md": "# R\n",
	}, "a.md", "guide/a.md")
	assert.Equal(t, "[p](../README.md)\n", got["a.md"])
}

// TestMove_FootnoteDefinitionsLeftAsWritten pins that a footnote
// definition, which the parser reads as a reference definition with a
// `^` label, is never rewritten: its text is not a destination. Only a
// link inside longer footnote text is repointed, in the moved file and
// in a file that points at it.
func TestMove_FootnoteDefinitionsLeftAsWritten(t *testing.T) {
	got := moveAndApply(t, map[string]string{
		"docs/a.md": "T[^1][^2][^3].\n\n[^1]: Ibid.\n[^2]: [z](x.md)\n[r]: x.md\n\n[^3]: See [y](x.md).\n",
		"docs/x.md": "# X\n",
		"b.md":      "T[^n] [a][r].\n\n[^n]: docs/a.md\n[r]: docs/a.md\n",
	}, "docs/a.md", "guide/a.md")
	assert.Equal(t,
		"T[^1][^2][^3].\n\n[^1]: Ibid.\n[^2]: [z](x.md)\n[r]: ../docs/x.md\n\n[^3]: See [y](../docs/x.md).\n",
		got["docs/a.md"])
	assert.Equal(t, "T[^n] [a][r].\n\n[^n]: docs/a.md\n[r]: guide/a.md\n", got["b.md"])
}

// TestMove_NonMarkdownSourceBytesUntouched pins that a moved file
// without a Markdown extension, such as an image or a text file, keeps
// its bytes: link-shaped bytes in it are not destinations. The links
// that name it are still repointed.
func TestMove_NonMarkdownSourceBytesUntouched(t *testing.T) {
	got := moveAndApply(t, map[string]string{
		"docs/n.txt": "see [z](x.md)\n",
		"docs/x.md":  "# X\n",
		"docs/b.md":  "[n](n.txt)\n",
	}, "docs/n.txt", "guide/n.txt")
	assert.Equal(t, "see [z](x.md)\n", got["docs/n.txt"])
	assert.Equal(t, "[n](../guide/n.txt)\n", got["docs/b.md"])
}

// TestMove_SelfRefDefFollowsRenamedFile pins that the moved file's
// ref-def to itself follows a move that changes both the directory and
// the basename, spelled from the new directory.
func TestMove_SelfRefDefFollowsRenamedFile(t *testing.T) {
	got := moveAndApply(t, map[string]string{"a.md": "[s][self]\n\n[self]: ./a.md#top\n"}, "a.md", "d/b.md")
	assert.Equal(t, "[s][self]\n\n[self]: ./b.md#top\n", got["a.md"])
}
