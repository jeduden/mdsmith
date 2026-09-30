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
