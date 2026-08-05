package export

import (
	"fmt"
	"strings"
	"testing"
)

// normalizeBlankLinesFixture is a representative export input: prose
// paragraphs interleaved with intentional blank-line runs, sized to
// dominate the per-call cost normalizeBlankLines pays.
func normalizeBlankLinesFixture() []byte {
	var b strings.Builder
	for i := 0; i < 60; i++ {
		fmt.Fprintf(&b, "Paragraph %d with enough words to look like real prose content.\n\n\n", i)
	}
	return []byte(b.String())
}

// TestNormalizeBlankLinesAllocBudget pins the allocation cost of
// normalizeBlankLines. It converted the entire input to a string via
// `string(src)` before splitting, allocating a full copy of the file
// just to drive strings.Split, when the function's job is line-oriented
// byte scanning throughout (docs/development/high-performance-go.md
// "Stay in []byte").
func TestNormalizeBlankLinesAllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race; the race detector " +
			"adds allocation bookkeeping that perturbs the count")
	}
	src := normalizeBlankLinesFixture()
	codeBlockLines := map[int]struct{}{}

	out := normalizeBlankLines(src, codeBlockLines)
	if len(out) == 0 {
		t.Fatal("normalizeBlankLines returned empty output for non-empty input")
	}

	const runs = 100
	allocs := testing.AllocsPerRun(runs, func() {
		_ = normalizeBlankLines(src, codeBlockLines)
	})
	const budget = 10
	t.Logf("normalizeBlankLines allocs/op on a %d-byte input = %.0f (budget = %d)",
		len(src), allocs, budget)
	if allocs > float64(budget) {
		t.Fatalf("normalizeBlankLines allocs/op = %.0f, budget = %d; "+
			"it should split/scan []byte directly instead of converting "+
			"the whole input to a string first (see "+
			"docs/development/high-performance-go.md)",
			allocs, budget)
	}
}
