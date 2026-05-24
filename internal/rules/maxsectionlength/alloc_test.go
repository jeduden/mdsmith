package maxsectionlength

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// allocBudgetMDS036 mirrors the published per-Check ceiling
// (CLAUDE.md ≤ 10 allocs on representative input). The opt-in
// configuration MDS036 ships with — every knob zero — must not pay
// for the heading and paragraph AST walks: skipping them on the
// configured-no-knobs path is plan 195 task 12's fix.
const allocBudgetMDS036 = 10

// allocBudgetFixture is the same representative document the
// integration alloc-budget gate uses, so a regression that the
// matrix would catch also fails here from a single package.
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
// the fixture. Mirrors the shape used by other rules' alloc gates so
// the same number is reproduced by the integration matrix.
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

// TestCheckAllocBudget_NoLimits pins MDS036 at 0 allocs/op when no
// knob is configured. The opt-in default ships with every limit
// zero, so the rule must not walk the AST for headings or
// paragraphs on that path.
func TestCheckAllocBudget_NoLimits(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	r := &Rule{}
	allocs := checkAllocsPerOp(t, r)
	t.Logf("MDS036 (no-knobs) Check allocs/op = %.0f", allocs)
	require.LessOrEqualf(t, allocs, float64(allocBudgetMDS036),
		"MDS036 (no-knobs) Check allocs/op = %.0f, ceiling = %d",
		allocs, allocBudgetMDS036)
}

// TestCheck_NoLimits_NoWork pins the no-knobs early exit: the rule
// returns nil without scanning the AST when every limit is zero.
// Holds even on a document with headings that would otherwise have
// been walked.
func TestCheck_NoLimits_NoWork(t *testing.T) {
	f := mustFile(t, "# H1\nbody\n## H2\nmore\n")
	r := &Rule{}
	require.Nil(t, r.Check(f))
}
