package blanklinearoundheadings

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
)

// fixAllocBudget is the maximum heap allocations Fix may perform on a
// small document that requires blank-line insertions. The budget
// accounts for two set maps (insertBefore/insertAfter), the
// CollectCodeBlockLines memo, the AST-walk closure, one pre-sized
// result slice, and the bytes.Join output. The previous O(n)
// string([]byte) per-line conversion pattern allocated ~23 objects for
// the 6-line fixture; the refactored code targets ≤ 8.
const fixAllocBudget = 8

// fixAllocFixture is a 6-line document that requires four blank-line
// insertions so the Fix path exercises all insertion branches.
const fixAllocFixture = "# Title\nSome text\n## Section\nMore text\n## Final\nEnd.\n"

func TestFixAllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race; the race detector perturbs allocation counts")
	}
	src := []byte(fixAllocFixture)
	r := &Rule{}

	// Warm: ensure no first-run overhead charges to the measured frame.
	warm, err := lint.NewFile("warm.md", src)
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Fix(warm)

	const runs = 100
	parse := testing.AllocsPerRun(runs, func() {
		f, err := lint.NewFile("parse.md", src)
		if err != nil {
			t.Fatal(err)
		}
		_ = f
	})
	full := testing.AllocsPerRun(runs, func() {
		f, err := lint.NewFile("fix.md", src)
		if err != nil {
			t.Fatal(err)
		}
		_ = r.Fix(f)
	})
	delta := full - parse
	if delta < 0 {
		delta = 0
	}
	t.Logf("Fix allocs/op = %.1f (budget = %d)", delta, fixAllocBudget)
	if delta > float64(fixAllocBudget) {
		t.Fatalf(
			"Fix allocs/op = %.1f exceeds budget %d; "+
				"check for O(n) string([]byte) per-line conversions in Fix "+
				"(docs/development/high-performance-go.md §Strings and bytes)",
			delta, fixAllocBudget)
	}
}
