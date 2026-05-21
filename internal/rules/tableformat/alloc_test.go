package tableformat

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// allocBudgetMDS025 is the per-rule ceiling MDS025 must reach when
// the shared `tablefmt` cell-as-byte-offsets refactor (deferred in
// plan 195 task 3) lands. The integration gate grandfathers the
// in-progress baseline; the unit-test budget below tracks today's
// number so a regression fails CI even before the refactor lands.
// Kept in lockstep with allocBudgetGrandfathered["MDS025"] in
// internal/integration/alloc_budget_test.go — a single
// authoritative baseline per rule.
const (
	allocBudgetMDS025              = 10
	allocBudgetGrandfatheredMDS025 = 110
)

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

func TestCheckAllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	r := &Rule{}
	src := []byte(allocBudgetFixture)
	warm, err := lint.NewFile("warm.md", src)
	require.NoError(t, err)
	_ = r.Check(warm)
	const runs = 100
	parse := testing.AllocsPerRun(runs, func() {
		_, err := lint.NewFile("parse.md", src)
		require.NoError(t, err)
	})
	full := testing.AllocsPerRun(runs, func() {
		f, err := lint.NewFile("check.md", src)
		require.NoError(t, err)
		_ = r.Check(f)
	})
	allocs := full - parse
	if allocs < 0 {
		allocs = 0
	}
	t.Logf("MDS025 Check allocs/op = %.0f (target = %d, grandfathered = %d)",
		allocs, allocBudgetMDS025, allocBudgetGrandfatheredMDS025)
	require.LessOrEqualf(t, allocs, float64(allocBudgetGrandfatheredMDS025),
		"MDS025 Check allocs/op = %.0f, grandfathered = %d (plan 195 task 3)",
		allocs, allocBudgetGrandfatheredMDS025)
}
