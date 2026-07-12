package ambiguousemphasis

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// allocBudgetMDS047 mirrors the CLAUDE.md per-Check ceiling ("A
// rule's Check allocates ≤ 10 times per call on representative
// input") on ordinary prose that actually uses emphasis (**bold**,
// *italic*, _underscore_) — the shape that dominates real documents.
// The shared integration alloc-budget fixture
// (internal/integration/alloc_budget_test.go) carries no emphasis at
// all, so it never exercised this cost and the rule reported 0
// allocs/op on BenchmarkPerRuleAllocBudget despite scanLine
// allocating a fresh *emphRun per delimiter run (~10 allocs/line on
// emphasis-heavy prose) — see docs/development/high-performance-go.md
// "Gate expensive analyzers behind a cheap pre-check" and "Fixed-size
// arrays beat slices". scanLine now gates on a cheap byte-count
// pre-check and tracks the in-progress run by value instead of by
// pointer; escapedInRunDiags and adjacentSameDelimDiags skip building
// their tracking maps when no escape or fewer than three runs can
// possibly produce a diagnostic; and Check no longer pre-sizes diags
// for a violation rate real documents rarely hit. Landed at 9
// allocs/op on this fixture (was 41 before any of these fixes).
const allocBudgetMDS047 = 10

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

// TestCheckAllocBudget_EmphasisHeavyProse pins MDS047 at <= 10
// allocs/op (CLAUDE.md's documented ceiling) on a short document
// that actually uses emphasis, with every detector active. Skipped
// under -race and -short, matching the integration gate.
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
