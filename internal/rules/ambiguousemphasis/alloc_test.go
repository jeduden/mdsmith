package ambiguousemphasis

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// allocBudgetMDS047 pins Check's allocation cost on ordinary prose
// that actually uses emphasis (**bold**, *italic*, _underscore_) —
// the shape that dominates real documents. The shared integration
// alloc-budget fixture (internal/integration/alloc_budget_test.go)
// carries no emphasis at all, so it never exercised this cost and
// the rule reported 0 allocs/op on BenchmarkPerRuleAllocBudget
// despite scanLine allocating a fresh *emphRun per delimiter run
// (~10 allocs/line on emphasis-heavy prose) — see
// docs/development/high-performance-go.md "Gate expensive analyzers
// behind a cheap pre-check" and "Fixed-size arrays beat slices".
// scanLine now gates on a cheap bytes.Count pre-check and tracks the
// in-progress run by value instead of by pointer, landing at 1
// alloc/line (the pre-sized runs slice) on this fixture.
const allocBudgetMDS047 = 15

// allocBudgetFixtureMDS047 is a representative prose document that
// actually contains emphasis, unlike the shared integration fixture.
const allocBudgetFixtureMDS047 = "# Document title\n" +
	"\n" +
	"A short prose paragraph with **bold** text, *italic* text, and\n" +
	"some _underscore_ emphasis too. It stays a few lines long, the\n" +
	"way *real* Markdown reads.\n" +
	"\n" +
	"## Section\n" +
	"\n" +
	"See [other](other.md) for **more** examples of *emphasis* in the\n" +
	"wild, including _nested_ cases and **double** markers.\n" +
	"\n" +
	"- one **bold** item\n" +
	"- two *italic* items\n"

// checkAllocsPerOpMDS047 returns parse-subtracted allocs/op for
// r.Check on src. Fresh File per iteration so per-File memos start
// cold, matching the integration gate's shape.
func checkAllocsPerOpMDS047(tb testing.TB, r *Rule, src []byte) float64 {
	tb.Helper()
	warm, err := lint.NewFile("warm.md", src)
	require.NoError(tb, err)
	_ = r.Check(warm)
	const runs = 100
	parse := testing.AllocsPerRun(runs, func() {
		_, err := lint.NewFile("parse.md", src)
		require.NoError(tb, err)
	})
	full := testing.AllocsPerRun(runs, func() {
		f, err := lint.NewFile("check.md", src)
		require.NoError(tb, err)
		_ = r.Check(f)
	})
	delta := full - parse
	if delta < 0 {
		delta = 0
	}
	return delta
}

// TestCheckAllocBudget_EmphasisHeavyProse pins MDS047 at <= 20
// allocs/op on a short document that actually uses emphasis, with
// every detector active. Skipped under -race and -short, matching
// the integration gate.
func TestCheckAllocBudget_EmphasisHeavyProse(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race; the race detector " +
			"adds allocation bookkeeping that perturbs the count")
	}
	r := activeRule()
	src := []byte(allocBudgetFixtureMDS047)
	allocs := checkAllocsPerOpMDS047(t, r, src)
	t.Logf("MDS047 Check allocs/op on emphasis-heavy prose = %.0f (budget = %d)",
		allocs, allocBudgetMDS047)
	require.LessOrEqualf(t, allocs, float64(allocBudgetMDS047),
		"MDS047 Check allocs/op = %.0f, budget = %d; see "+
			"docs/development/high-performance-go.md",
		allocs, allocBudgetMDS047)
}
