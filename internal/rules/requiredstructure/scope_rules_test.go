package requiredstructure

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/schema"
	"github.com/stretchr/testify/assert"
)

// TestScopeEndLine_BreaksAtParentEnd covers the
// `heads[j].Line >= parentEnd` branch: when the next heading
// after `matched` falls outside the parent window, the scan
// stops at parentEnd rather than reporting that heading's line
// (which would belong to a sibling scope).
func TestScopeEndLine_BreaksAtParentEnd(t *testing.T) {
	heads := []schema.DocHeading{
		{Level: 2, Text: "Matched", Line: 10},
		{Level: 3, Text: "Inside", Line: 15},
		// 25 is past parentEnd=20 — must not be reported.
		{Level: 2, Text: "Outside", Line: 25},
	}
	got := scopeEndLine(heads, 0, 2, 20)
	assert.Equal(t, 20, got,
		"a heading past parentEnd must not close the scope; the "+
			"function must fall through to the parentEnd return")
}

// TestFirstHeadingLine_SkipsOutOfRange covers the range skip and
// the no-match fallthrough: a heading before parentStart or at
// or past parentEnd is ignored, and when no heading inside the
// window matches expectedLevel the function returns parentEnd.
func TestFirstHeadingLine_SkipsOutOfRange(t *testing.T) {
	heads := []schema.DocHeading{
		// Before parentStart=5 — skipped.
		{Level: 2, Text: "Early", Line: 1},
		// In window but wrong level — skipped by the
		// `h.Level == expectedLevel` filter.
		{Level: 3, Text: "Deep", Line: 7},
		// At or past parentEnd=10 — skipped.
		{Level: 2, Text: "Late", Line: 10},
	}
	got := firstHeadingLine(heads, 2, 5, 10)
	assert.Equal(t, 10, got,
		"no in-window heading at expectedLevel → return parentEnd")
}

// TestIsSlotScope_NilMatcher covers isSlotScope's defensive nil
// branch: a preamble-shaped scope (no matcher) is not a slot.
func TestIsSlotScope_NilMatcher(t *testing.T) {
	assert.False(t, isSlotScope(schema.Scope{Preamble: true}),
		"a preamble scope has no matcher and is not a slot")
}
