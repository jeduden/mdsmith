package linelength

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// allocBudgetMDS001 is the per-Check ceiling MDS001 must fit
// (CLAUDE.md ≤ 10 per call on representative input). Measured on
// cold lint.File minus parse baseline, mirroring plan 193's
// BenchmarkRule_MDS024 shape so the two gates use the same yardstick.
const allocBudgetMDS001 = 10

// allocBudgetFixture is one small Markdown file: heading, prose,
// fenced code, link, list, table — the same shape the integration
// gate in internal/integration/alloc_budget_test.go uses, so a
// regression that the integration matrix would flag also fails
// here as a unit test (cheaper to bisect from a single rule
// package). Every line stays under 80 chars so MDS001 has nothing
// to flag; the gate measures the rule's base scanning cost on a
// compliant representative file.
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
// the shared fixture. Fresh lint.File per iteration so per-File
// memos start cold — matches production where File is per-Check.
func checkAllocsPerOp(tb testing.TB, r *Rule, body string) float64 {
	tb.Helper()
	src := []byte(body)
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

// TestCheckAllocBudget pins MDS001 at ≤ 10 allocs/op on the
// representative fixture. Skipped under -race (race detector
// bookkeeping perturbs the count) and -short. Plan 195 task 4 is
// the fix that brings the rule under the ceiling.
func TestCheckAllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	allocs := checkAllocsPerOp(t, r, allocBudgetFixture)
	t.Logf("MDS001 Check allocs/op = %.0f (budget = %d)",
		allocs, allocBudgetMDS001)
	require.LessOrEqualf(t, allocs, float64(allocBudgetMDS001),
		"MDS001 Check allocs/op = %.0f, budget = %d (plan 195)",
		allocs, allocBudgetMDS001)
}
