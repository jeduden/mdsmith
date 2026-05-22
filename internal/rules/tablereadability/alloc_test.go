package tablereadability

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// allocBudgetMDS026 is the per-rule ceiling MDS026 must reach when
// the cell-as-byte-offsets refactor (deferred in plan 195 task 2)
// lands. The integration gate at
// internal/integration/alloc_budget_test.go grandfathers the
// in-progress baseline so CI stays green while the rest of the
// refactor is scheduled; this unit-test budget tracks the final
// target. allocBudgetGrandfatheredMDS026 is the present-day
// baseline the gate refuses to regress past. Kept in lockstep
// with allocBudgetGrandfathered["MDS026"] in
// internal/integration/alloc_budget_test.go — a single
// authoritative baseline per rule.
const (
	allocBudgetMDS026              = 10
	allocBudgetGrandfatheredMDS026 = 18
)

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
	t.Logf("MDS026 Check allocs/op = %.0f (target = %d, grandfathered = %d)",
		allocs, allocBudgetMDS026, allocBudgetGrandfatheredMDS026)
	// While the cell-as-byte-offsets refactor is pending, the unit
	// gate enforces the grandfathered baseline so any regression
	// past today's number still fails. Once the refactor lands,
	// drop allocBudgetGrandfatheredMDS026 and tighten back to
	// allocBudgetMDS026.
	require.LessOrEqualf(t, allocs, float64(allocBudgetGrandfatheredMDS026),
		"MDS026 Check allocs/op = %.0f, grandfathered = %d (plan 195 task 2)",
		allocs, allocBudgetGrandfatheredMDS026)
}
