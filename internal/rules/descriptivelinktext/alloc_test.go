package descriptivelinktext

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// allocBudgetMDS063 mirrors the CLAUDE.md per-Check ceiling. Plan
// 195 task 9 lifts the bannedSet build out of the per-File memo and
// onto the rule instance; the gate fixture's standalone Check now
// pays four allocations (mostly the WalkNodes closure and the per-
// link text walk) instead of the ~17 the per-File memo build paid.
const allocBudgetMDS063 = 10

// allocBudgetFixture is the integration alloc-budget fixture
// duplicated here so the per-rule unit gate catches a regression
// before the matrix would.
const allocBudgetFixture = "# Document title\n" +
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

// checkAllocsPerOp returns parse-subtracted allocs/op for r.Check on
// the fixture. Fresh File per iteration so per-File memos start cold
// — matches the integration gate's shape.
func checkAllocsPerOp(tb testing.TB, r *Rule) float64 {
	tb.Helper()
	src := []byte(allocBudgetFixture)
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

// TestCheckAllocBudget pins MDS063 at ≤ 10 allocs/op on the
// representative fixture. Skipped under -race and -short, matching
// the integration gate.
func TestCheckAllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	r := &Rule{Banned: append([]string(nil), defaultBanned...)}
	allocs := checkAllocsPerOp(t, r)
	t.Logf("MDS063 Check allocs/op = %.0f (budget = %d)",
		allocs, allocBudgetMDS063)
	require.LessOrEqualf(t, allocs, float64(allocBudgetMDS063),
		"MDS063 Check allocs/op = %.0f, budget = %d (plan 195)",
		allocs, allocBudgetMDS063)
}
