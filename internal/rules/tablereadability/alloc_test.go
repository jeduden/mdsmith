package tablereadability

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// allocBudgetMDS026 is the per-rule ceiling MDS026 must stay under.
// The cell-as-byte-offsets refactor (plan 195 task 2) landed: the
// splitRow path returns [][]byte sub-slices into the source line,
// and the readability counters consume them via bytes.TrimSpace /
// utf8.RuneCount / countWords — no per-cell string alloc. The
// integration gate no longer grandfathers MDS026.
const allocBudgetMDS026 = 10

// allocBudgetFixture mirrors the integration gate at
// internal/integration/alloc_budget_test.go — heading, prose, code
// fence, links, list, table, ref. Every line stays under 80 chars
// so MDS001 has nothing to flag elsewhere; this fixture is the
// per-rule mirror so a regression that the integration matrix
// would flag also fails here as a unit test (cheaper to bisect
// from one rule package).
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

func TestCheckAllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	r := &Rule{
		MaxColumns:          defaultMaxColumns,
		MaxRows:             defaultMaxRows,
		MaxWordsPerCell:     defaultMaxWordsPerCell,
		MaxColumnWidthRatio: defaultMaxColumnWidthRatio,
	}
	allocs := checkAllocsPerOp(t, r, allocBudgetFixture)
	t.Logf("MDS026 Check allocs/op = %.0f (budget = %d)",
		allocs, allocBudgetMDS026)
	require.LessOrEqualf(t, allocs, float64(allocBudgetMDS026),
		"MDS026 Check allocs/op = %.0f, budget = %d (plan 195)",
		allocs, allocBudgetMDS026)
}
