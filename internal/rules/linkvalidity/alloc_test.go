package linkvalidity

import (
	"testing"
)

// allocBudgetReversedInLine is the per-call ceiling for reversedInLine
// on a line with several reversed-link matches. idx (the regex
// submatch-index slice) already gives the exact match count before the
// result loop starts, so out's final size is known up front — see
// docs/development/high-performance-go.md "Pre-size slices."
const allocBudgetReversedInLine = 6

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
