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
