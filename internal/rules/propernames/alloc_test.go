package propernames

import (
	"fmt"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// allocBudgetMDS050 pins Check's steady-state allocation cost when
// the rule is configured with real names. buildNameEntries()
// rebuilds every nameEntry (a fresh []byte plus an asciiToLower copy
// per name) on every single Check call, even though r.Names is
// immutable for the life of the Rule instance — a per-Rule
// memoization case, not a per-file one
// (docs/development/high-performance-go.md "Memoize per-input
// computations"). The shared integration alloc-budget fixture never
// sets Names, so len(entries)==0 short-circuits before any of this
// runs and MDS050 reports 0 allocs/op on
// BenchmarkPerRuleAllocBudget, leaving the real per-Check cost
// invisible to CI.
const allocBudgetMDS050 = 10

// allocBudgetFixtureMDS050 is the shared integration alloc-budget
// fixture (internal/integration/alloc_budget_test.go), duplicated
// here so the per-rule unit gate catches a regression before the
// matrix would.
const allocBudgetFixtureMDS050 = "# Document title\n" +
	"\n" +
	"A short prose paragraph for the readability and structural\n" +
	"rules to scan. It stays one paragraph long.\n" +
	"\n" +
	"## Section\n" +
	"\n" +
	"See [other](other.md) and [label][ref] for examples.\n" +
	"\n" +
	"```go\nfunc f() int { return 0 }\n```\n" +
	"\n" +
	"- one item\n" +
	"- two items\n" +
	"\n" +
	"| Col | Other |\n" +
	"|-----|-------|\n" +
	"| a   | b     |\n" +
	"\n" +
	"[ref]: https://example.com/\n"

// checkAllocsPerOpMDS050 returns parse-subtracted allocs/op for
// r.Check on the fixture. r (not just the File) is reused across
// every measured iteration, matching what the engine does across a
// workspace walk: one configured Rule instance checks many files.
// A cold-cache warm-up call runs first so the measured loop reflects
// the steady-state per-file cost.
func checkAllocsPerOpMDS050(tb testing.TB, r *Rule) float64 {
	tb.Helper()
	src := []byte(allocBudgetFixtureMDS050)
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

// TestCheckAllocBudget pins MDS050 at <= 10 allocs/op on the
// representative fixture once configured with a realistic 20-name
// vocabulary. Skipped under -race and -short, matching the
// integration gate.
func TestCheckAllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race; the race detector " +
			"adds allocation bookkeeping that perturbs the count")
	}
	names := make([]string, 20)
	for i := range names {
		names[i] = fmt.Sprintf("ProperName%d", i)
	}
	r := &Rule{Names: names}
	allocs := checkAllocsPerOpMDS050(t, r)
	t.Logf("MDS050 Check allocs/op = %.0f (budget = %d)",
		allocs, allocBudgetMDS050)
	require.LessOrEqualf(t, allocs, float64(allocBudgetMDS050),
		"MDS050 Check allocs/op = %.0f, budget = %d; see "+
			"docs/development/high-performance-go.md",
		allocs, allocBudgetMDS050)
}
