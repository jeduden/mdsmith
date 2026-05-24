//go:build !goldmark_upstream

package parser

import "github.com/yuin/goldmark/arena"

// newArenaForParse returns a fresh arena.Arena. Each Parse call
// gets its own arena so AST lifetime is bounded by GC of the
// returned tree, not by the next Parse on the same Parser — the
// pool-safety concern called out in plan 198's risk section.
//
// Built without the `goldmark_upstream` tag (the canonical path).
func newArenaForParse() *arena.Arena {
	return arena.New()
}
