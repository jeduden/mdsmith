//go:build !mdtext_punkt_upstream

// The allocation budget is owned by the default mdsmith build, which
// dispatches the internal/punkt fork from mdtext. The upstream tag
// (mdtext_punkt_upstream) deliberately keeps the original
// neurosnap/sentences pipeline around for A/B segmentation
// comparison; that pipeline does not respect the budget, so the
// alloc-gate tests below would always fail under that tag. Skipping
// at compile time keeps the upstream-build CI lane green for the
// segmentation tests it does cover.
package paragraphstructure

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/testcorpus"
	"github.com/stretchr/testify/require"
)

// allocBudgetMDS024 is the per-Check allocation ceiling enforced by
// BenchmarkRule_MDS024 and TestCheckAllocBudget. The target lives in
// CLAUDE.md ("A rule's Check allocates ≤ 10 times per call on
// representative input") and is the acceptance criterion of plan 193.
//
// Measured baseline on testcorpus.AbbrHeavyParagraph after the
// internal/punkt rework: 7 allocs/op on a warm File (parse and
// per-File memos excluded — the engine pays those once per file, not
// per rule). The budget pins the rule under the CLAUDE.md ceiling
// with three allocs of headroom.
const allocBudgetMDS024 = 10

// checkAllocsPerOp returns the allocs/op of one Check call against a
// preconstructed lint.File. Shared by BenchmarkRule_MDS024 and
// TestCheckAllocBudget so the budget gate fires under both `go test`
// and `go test -bench`.
//
// The lint.NewFile call lives outside the measured closure on
// purpose: parse-time allocations belong to the engine, not the rule,
// and the per-File memos (AST, section paragraphs) are warm by the
// time the engine dispatches the rule. The plan's ≤ 10 allocs budget
// targets the same hot path — the Check call itself against a parsed,
// memo-warmed File.
func checkAllocsPerOp(tb testing.TB, r *Rule, body string) float64 {
	tb.Helper()
	src := []byte(body + "\n")
	f, err := lint.NewFile("bench.md", src)
	require.NoError(tb, err)
	// Warm any lazy init (Punkt training, memoized paragraph walks)
	// once outside the measured run so init cost is not charged here.
	_ = r.Check(f)
	return testing.AllocsPerRun(50, func() {
		_ = r.Check(f)
	})
}

// BenchmarkRule_MDS024 reports allocs/op, B/op, and ns/op for one
// Check call against a realistic abbreviation-heavy paragraph. The
// fixture is testcorpus.AbbrHeavyParagraph — the same bytes
// BenchmarkSplitSentences_Subset uses, joined into one paragraph so
// MDS024's per-paragraph code path is what the gate measures.
//
// A b.Fatalf trips when allocs/op exceeds allocBudgetMDS024 so a
// regression that pushes the rule back over the ceiling fails CI
// instead of silently degrading. See plan 193 task 1 (the gate
// added here) and task 12 (the ceiling tightening to 10).
func BenchmarkRule_MDS024(b *testing.B) {
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	body := testcorpus.AbbrHeavyParagraph()
	src := []byte(body + "\n")
	f, err := lint.NewFile("bench.md", src)
	require.NoError(b, err)
	_ = r.Check(f) // warm lazy init and per-File memos

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = r.Check(f)
	}
	b.StopTimer()

	allocs := checkAllocsPerOp(b, r, body)
	b.ReportMetric(allocs, "allocs/check")
	if allocs > float64(allocBudgetMDS024) {
		b.Fatalf("MDS024 Check allocs/op = %.0f, budget = %d; "+
			"see plan 193 for the rationale and the breakdown",
			allocs, allocBudgetMDS024)
	}
}

// TestCheckAllocBudget pins the per-call allocation count under a
// normal `go test` run, not only under `-bench`. The ceiling is the
// same one BenchmarkRule_MDS024 enforces, so a regression that lands
// without `-bench` still trips. Skipped under `-short` because the
// underlying AllocsPerRun runs the closure 50+ times, and skipped
// under `-race` because the race detector adds enough allocation
// bookkeeping to make the ≤ 10 budget flaky.
func TestCheckAllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race; the race detector " +
			"adds allocation bookkeeping that perturbs the count")
	}
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	body := testcorpus.AbbrHeavyParagraph()
	allocs := checkAllocsPerOp(t, r, body)
	t.Logf("MDS024 Check allocs/op = %.0f (budget = %d)",
		allocs, allocBudgetMDS024)
	require.LessOrEqualf(t, allocs, float64(allocBudgetMDS024),
		"MDS024 Check allocs/op = %.0f, budget = %d (see plan 193)",
		allocs, allocBudgetMDS024)
}
