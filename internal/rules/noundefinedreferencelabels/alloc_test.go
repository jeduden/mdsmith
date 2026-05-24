package noundefinedreferencelabels

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// allocBudgetMDS054 is the per-rule ceiling MDS054 must stay under.
// Plan 195 task 7 landed: collectNormalisedDefs returns a sized
// []string instead of a map, labelDefined linear-scans it, the
// no-bracket early-exit short-circuits prose files, and the
// `len(r.Placeholders) > 0 &&` guard avoids the per-match
// `string(label)` cast when no placeholder vocabulary is
// configured (the default).
const allocBudgetMDS054 = 10

// allocBudgetFixture mirrors the integration alloc-budget fixture
// at internal/integration/alloc_budget_test.go so the unit-level
// gate catches a regression from a single package without booting
// the full rule matrix.
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

// TestCheckAllocBudget pins MDS054's per-Check allocation count to
// the ≤ 10 ceiling. Skipped under -race and -short, matching the
// integration matrix.
func TestCheckAllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	r := &Rule{}
	allocs := checkAllocsPerOp(t, r)
	t.Logf("MDS054 Check allocs/op = %.0f (budget = %d)",
		allocs, allocBudgetMDS054)
	require.LessOrEqualf(t, allocs, float64(allocBudgetMDS054),
		"MDS054 Check allocs/op = %.0f, budget = %d (plan 195)",
		allocs, allocBudgetMDS054)
}
