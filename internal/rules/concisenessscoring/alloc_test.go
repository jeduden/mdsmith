package concisenessscoring

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// allocBudgetMDS029 mirrors the published per-Check ceiling. Plan
// 195 task 13 short-circuits paragraphs whose word count is below
// MinWords before running the classifier; the classifier's
// regex-driven cue extraction was the dominant allocator (~400
// allocs/Check on the alloc-budget fixture's single sub-MinWords
// paragraph).
const allocBudgetMDS029 = 10

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

func TestCheckAllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	r := &Rule{MinScore: defaultMinScore, MinWords: defaultMinWords}
	allocs := checkAllocsPerOp(t, r)
	t.Logf("MDS029 Check allocs/op = %.0f (budget = %d)",
		allocs, allocBudgetMDS029)
	require.LessOrEqualf(t, allocs, float64(allocBudgetMDS029),
		"MDS029 Check allocs/op = %.0f, budget = %d (plan 195)",
		allocs, allocBudgetMDS029)
}
