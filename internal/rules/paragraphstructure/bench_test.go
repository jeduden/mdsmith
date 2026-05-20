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
// The ceiling is loose until plan 193 tasks 2–10 land. Per-paragraph
// SplitSentences on the abbr-heavy corpus is ~135 allocs (plan 191
// numbers); MDS024.Check additionally pays for NewFile parse and AST
// walk, so the measured baseline on testcorpus.AbbrHeavyParagraph is
// ~1100 allocs/op. Task 12 flips this constant to 10 once the
// internal/punkt rework is in place. Until then the smoke ceiling
// sits above the baseline so the gate fails only on a regression,
// not on the unmigrated rule.
const allocBudgetMDS024 = 1500

// checkAllocsPerOp returns the allocs/op of one Check call against
// body, parsed as a fresh lint.File on every iteration. Shared by
// BenchmarkRule_MDS024 and TestCheckAllocBudget so the budget gate
// fires under both `go test` and `go test -bench`.
//
// The lint.NewFile call is inside the timed function on purpose:
// MDS024's production call site re-parses for every file, so a
// realistic per-call measurement includes the parse alongside the
// rule's own work. The parse is the same in every iteration, so its
// allocations stay constant; a rule-side regression still surfaces.
func checkAllocsPerOp(tb testing.TB, r *Rule, body string) float64 {
	tb.Helper()
	src := []byte(body + "\n")
	// Warm any lazy init (Punkt training data, package-scope regexes)
	// once outside AllocsPerRun so init cost is not charged to the
	// measured allocs.
	warm, err := lint.NewFile("warm.md", src)
	require.NoError(tb, err)
	_ = r.Check(warm)
	return testing.AllocsPerRun(50, func() {
		f, err := lint.NewFile("bench.md", src)
		require.NoError(tb, err)
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
	warm, err := lint.NewFile("warm.md", src)
	require.NoError(b, err)
	_ = r.Check(warm)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		f, err := lint.NewFile("bench.md", src)
		require.NoError(b, err)
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
// underlying AllocsPerRun runs the closure 50+ times.
func TestCheckAllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
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
