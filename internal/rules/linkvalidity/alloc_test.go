package linkvalidity

import (
	"testing"
)

// allocBudgetReversedInLine is the per-call ceiling for reversedInLine
// on a line with several reversed-link matches. idx (the regex
// submatch-index slice) already gives the exact upper bound on match
// count before the result loop starts, so out's final size is knowable
// up front — see docs/development/high-performance-go.md "Pre-size
// slices." Baseline measured 6; +4 headroom follows the project's
// "baseline plus max(20%, 4)" convention so an unrelated +1 (regexp
// internals, GOARCH) doesn't turn CI red.
const allocBudgetReversedInLine = 10

// reversedInLineMultiMatchFixture has four reversed-link matches on one
// line, so an unpresized `out` (starting nil, doubling via append) pays
// extra grow-and-copy allocations a presized make([]revMatch, 0,
// len(idx)) would not.
var reversedInLineMultiMatchFixture = []byte(
	"(one)[a] (two)[b] (three)[c] (four)[d]",
)

func TestReversedInLineAllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	line := reversedInLineMultiMatchFixture

	_ = reversedInLine(line, line) // warm up regex program cache
	allocs := testing.AllocsPerRun(200, func() {
		_ = reversedInLine(line, line)
	})
	t.Logf("reversedInLine allocs/op = %.0f (budget = %d)", allocs, allocBudgetReversedInLine)
	if allocs > float64(allocBudgetReversedInLine) {
		t.Fatalf("reversedInLine allocs/op = %.0f, budget = %d: out must be presized with "+
			"make([]revMatch, 0, len(idx)) instead of growing via bare append",
			allocs, allocBudgetReversedInLine)
	}
}

// reversedInLineAllFilteredFixture has a regex match (an ordinary
// [text](url)[ref] — a link immediately followed by a reference or
// footnote) that the `]`-preceding-byte guard always drops, so
// reversedInLine must return nil, not a discarded non-nil empty slice,
// and must not pay for a slice it never uses.
var reversedInLineAllFilteredFixture = []byte(
	"see [text](https://example.com)[^1] for more",
)

// allocBudgetReversedInLineAllFiltered is the per-call ceiling on
// reversedInLineAllFilteredFixture: only reversedRe's own match-finding
// cost (no result slice, since every candidate is filtered). Measured
// 2; +4 headroom per the project's "baseline plus max(20%, 4)"
// convention. A version that presizes `out` unconditionally before the
// guard loop pays one extra allocation for a slice it then discards —
// this budget catches that regression on top of the nilness check
// below.
const allocBudgetReversedInLineAllFiltered = 6

// TestReversedInLineAllFilteredReturnsNil pins both the nilness and the
// zero-extra-alloc property of the all-matches-filtered path. A version
// that presizes `out` unconditionally before the guard loop allocates a
// slice it then discards and returns non-nil — a regression this test
// exists to catch.
func TestReversedInLineAllFilteredReturnsNil(t *testing.T) {
	line := reversedInLineAllFilteredFixture

	if got := reversedInLine(line, line); got != nil {
		t.Fatalf("reversedInLine on an all-filtered line = %#v, want nil", got)
	}

	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	_ = reversedInLine(line, line) // warm up regex program cache
	allocs := testing.AllocsPerRun(200, func() {
		_ = reversedInLine(line, line)
	})
	t.Logf("reversedInLine allocs/op (all-filtered) = %.0f (budget = %d)",
		allocs, allocBudgetReversedInLineAllFiltered)
	if allocs > float64(allocBudgetReversedInLineAllFiltered) {
		t.Fatalf("reversedInLine allocs/op on an all-filtered line = %.0f, budget = %d: out must "+
			"be allocated lazily on the first surviving match, not unconditionally before the "+
			"guard loop",
			allocs, allocBudgetReversedInLineAllFiltered)
	}
}
