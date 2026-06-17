package blanklinearoundlists

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// allocBudgetMDS014 is the per-Check ceiling for blank-line-around-lists.
// The rule uses nil-slice append (no unconditional allocation) so allocs
// are dominated by lint.CollectCodeBlockLines.
const allocBudgetMDS014 = 4

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
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	src := []byte(allocBudgetFixture)
	r := &Rule{}
	warm, err := lint.NewFile("warm.md", src)
	require.NoError(t, err)
	_ = r.Check(warm)

	const runs = 100
	parse := testing.AllocsPerRun(runs, func() {
		_, _ = lint.NewFile("parse.md", src)
	})
	full := testing.AllocsPerRun(runs, func() {
		f, err := lint.NewFile("check.md", src)
		require.NoError(t, err)
		_ = r.Check(f)
	})
	delta := full - parse
	if delta < 0 {
		delta = 0
	}
	t.Logf("MDS014 Check allocs/op = %.0f (budget = %d)", delta, allocBudgetMDS014)
	require.LessOrEqualf(t, delta, float64(allocBudgetMDS014),
		"MDS014 Check allocs/op = %.0f, budget = %d",
		delta, allocBudgetMDS014)
}
