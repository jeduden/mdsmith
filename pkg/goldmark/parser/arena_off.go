//go:build goldmark_upstream

package parser

import "github.com/yuin/goldmark/arena"

// newArenaForParse returns nil under the goldmark_upstream build
// tag, so every (*arena.Arena) method called from the parsing path
// falls back to the upstream constructor. CI uses this tag to lint
// the same sources through both the arena and the non-arena path;
// the equivalence harness diffs the two ASTs.
//
// Built with the `goldmark_upstream` tag.
func newArenaForParse() *arena.Arena {
	return nil
}
