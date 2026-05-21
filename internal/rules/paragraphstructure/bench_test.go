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

// allocBudgetMDS024 sits strictly below the CLAUDE.md ≤ 10 ceiling
// ("A rule's Check allocates ≤ 10 times per call on representative
// input"). The measurement is cold-File minus parse — a fresh
// lint.File per iteration with NewFile's allocations subtracted out.
// That counts the rule's own work plus any per-File memos it
// triggers (astutil.CollectSectionParagraphs, ExtractPlainText)
// without the engine's parse cost, matching what a single
// paragraph-aware rule contributes per file in production.
//
// "Representative input" is one abbreviation-heavy paragraph from
// testcorpus.AbbrHeavy. That fires the rule's diagnostic emission
// and exercises the segmenter's hot frame, while staying the size
// of a real Markdown paragraph (not the artificially long join of
// the whole corpus). Measured baseline after the internal/punkt
// rework and the per-rule sync.Pool for the segmenter result
// slice: 9 allocs/op.
const allocBudgetMDS024 = 9

// checkAllocsPerOp returns the rule's per-file cost on body when
// no other rule has touched the file yet — fresh lint.File on
// every iteration, parse-only baseline subtracted. The tokenizer
// singleton is warmed once outside the measured runs because that
// init fires once per process, not per file.
func checkAllocsPerOp(tb testing.TB, r *Rule, body string) float64 {
	tb.Helper()
	src := []byte(body + "\n")
	warm, err := lint.NewFile("warm.md", src)
	require.NoError(tb, err)
	_ = r.Check(warm)

	const runs = 100
	parseAllocs := testing.AllocsPerRun(runs, func() {
		_, err := lint.NewFile("p.md", src)
		require.NoError(tb, err)
	})
	fullAllocs := testing.AllocsPerRun(runs, func() {
		f, err := lint.NewFile("c.md", src)
		require.NoError(tb, err)
		_ = r.Check(f)
	})
	delta := fullAllocs - parseAllocs
	if delta < 0 {
		delta = 0
	}
	return delta
}

// BenchmarkRule_MDS024 reports allocs/op for one Check call on a
// representative abbreviation-heavy paragraph (the first entry of
// testcorpus.AbbrHeavy — one real-size Markdown paragraph, not the
// artificially long join of the whole corpus). Each iteration parses
// a fresh lint.File so per-File memos start cold; that matches
// production, where File is per-Check.
//
// A b.Fatalf trips when checkAllocsPerOp's parse-subtracted result
// exceeds allocBudgetMDS024 so a regression past the CLAUDE.md ≤ 10
// ceiling fails CI instead of silently degrading. See plan 193
// task 1 (the gate added here) and task 12 (the budget rationale).
func BenchmarkRule_MDS024(b *testing.B) {
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	body := testcorpus.AbbrHeavy()[0]
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
// without `-bench` still trips. Skipped under `-short` because
// AllocsPerRun runs the closure 100 times, and skipped under
// `-race` because the race detector adds enough allocation
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
	body := testcorpus.AbbrHeavy()[0]
	allocs := checkAllocsPerOp(t, r, body)
	t.Logf("MDS024 Check allocs/op = %.0f (budget = %d)",
		allocs, allocBudgetMDS024)
	require.LessOrEqualf(t, allocs, float64(allocBudgetMDS024),
		"MDS024 Check allocs/op = %.0f, budget = %d (see plan 193)",
		allocs, allocBudgetMDS024)
}
