package nobareurls

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// allocBudgetMDS012 mirrors the published per-Check ceiling (CLAUDE.md).
// On the alloc_budget fixture (no bare URLs) CheckNode is never reached,
// so the rule contributes 0 allocs beyond parse. The test pins that
// baseline so any regression from 0 is caught.
const allocBudgetMDS012 = 0

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
	t.Logf("MDS012 Check allocs/op = %.0f (budget = %d)", delta, allocBudgetMDS012)
	require.LessOrEqualf(t, delta, float64(allocBudgetMDS012),
		"MDS012 Check allocs/op = %.0f, budget = %d",
		delta, allocBudgetMDS012)
}
