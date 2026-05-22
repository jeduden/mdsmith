package tableformat

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

const allocBudgetMDS025 = 10

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
	t.Logf("MDS025 Check allocs/op = %.0f (budget = %d)",
		allocs, allocBudgetMDS025)
	require.LessOrEqualf(t, allocs, float64(allocBudgetMDS025),
		"MDS025 Check allocs/op = %.0f, budget = %d (plan 195)",
		allocs, allocBudgetMDS025)
}
