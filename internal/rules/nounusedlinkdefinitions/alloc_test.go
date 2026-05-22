package nounusedlinkdefinitions

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// allocBudgetMDS053 is the partial-fix ceiling MDS053 must fit while
// plan 195 task 6 lands. The full plan target is allocBudgetCeiling
// (10); today's number reflects the byte-scanner + raw-label
// refactor and the closure-free `Once` helpers in lint.File. The
// remaining headroom hangs on a goldmark internal allocator
// (parser.parseContext.References packs every reference into a
// fresh interface slice on each call), which the rule cannot
// influence without an AST-walk refactor that would re-cost the
// allocs elsewhere on this fixture. The unit gate pins the
// post-partial number so a regression past it fails CI before the
// integration matrix would.
const allocBudgetMDS053 = 11

// allocBudgetFixture mirrors the integration alloc-budget fixture
// so the unit-level gate catches a regression from a single
// package without booting the full rule matrix.
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
// the fixture. Fresh File per iteration so per-File memos start cold
// — matches the integration gate.
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

// TestCheckAllocBudget pins MDS053's per-Check allocation count to
// the partial-fix ceiling. Skipped under -race and -short, matching
// the integration matrix.
func TestCheckAllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	r := &Rule{}
	allocs := checkAllocsPerOp(t, r)
	t.Logf("MDS053 Check allocs/op = %.0f (budget = %d)",
		allocs, allocBudgetMDS053)
	require.LessOrEqualf(t, allocs, float64(allocBudgetMDS053),
		"MDS053 Check allocs/op = %.0f, budget = %d (plan 195)",
		allocs, allocBudgetMDS053)
}
