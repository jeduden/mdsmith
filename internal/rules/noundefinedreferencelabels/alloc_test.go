package noundefinedreferencelabels

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// allocBudgetMDS054 is the per-rule ceiling MDS054 must stay under.
// Plan 195 task 7 landed: collectNormalisedDefs returns a sized
// []string instead of a map, labelDefined linear-scans it, the
// no-bracket early-exit short-circuits prose files, and the
// `len(r.Placeholders) > 0 &&` guard skips the ContainsBodyToken call
// (and, since this PR, the zero-copy util.BytesToReadOnlyString view it
// takes its argument through) when no placeholder vocabulary is
// configured (the default) — see allocBudgetMDS054PlaceholdersMarginal
// below for the placeholders-configured case this budget does not cover.
const allocBudgetMDS054 = 10

// allocBudgetFixture mirrors the integration alloc-budget fixture
// at internal/integration/alloc_budget_test.go so the unit-level
// gate catches a regression from a single package without booting
// the full rule matrix.
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

// allocBudgetMDS054PlaceholdersMarginal is the ceiling on how many more
// allocations enabling a `placeholders:` vocabulary may add on top of
// the same fixture's no-placeholder Check cost — isolating the
// per-bracket-candidate placeholder check in scanFullRefs,
// scanCollapsedRefs, and scanShortcutRefs from the fixture's other,
// reference-count-proportional costs (normalizeLabel etc.), which scale
// with either code path and would otherwise swamp the comparison. The
// check reads label/text as a zero-copy string view (unsafe.String over
// the f.Source sub-slice, per docs/development/high-performance-go.md)
// instead of allocating a copy with string() for every candidate, so
// the marginal cost of turning placeholders on stays ~0.
const allocBudgetMDS054PlaceholdersMarginal = 1

// allocBudgetPlaceholderFixture has five full reference-style links so
// the per-match placeholder check (gated by len(r.Placeholders) > 0)
// runs five times per Check, making a per-match string() allocation in
// that path show up clearly against the budget.
// Reference labels are multiple bytes long (not the single-char "a"
// style) so a string() copy of the label actually allocates — Go's
// runtime returns a static, non-allocating string for a one-byte
// string(b) conversion, which would mask the very allocation this test
// exists to catch.
const allocBudgetPlaceholderFixture = "# Document title\n" +
	"\n" +
	"See [alpha][alpha-ref], [beta][beta-ref], [gamma][gamma-ref], " +
	"[delta][delta-ref], and [epsilon][epsilon-ref] for examples.\n" +
	"\n" +
	"[alpha-ref]: https://example.com/a\n" +
	"[beta-ref]: https://example.com/b\n" +
	"[gamma-ref]: https://example.com/c\n" +
	"[delta-ref]: https://example.com/d\n" +
	"[epsilon-ref]: https://example.com/e\n"

// checkAllocsPerOpOn is checkAllocsPerOp against an arbitrary source
// rather than the fixed allocBudgetFixture, reusing the same
// warm/parse/Check-delta measurement.
func checkAllocsPerOpOn(tb testing.TB, r *Rule, src []byte) float64 {
	tb.Helper()
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

// TestCheckAllocBudgetWithPlaceholders pins how many additional
// allocations turning on a `placeholders:` vocabulary may cost, over
// the same fixture's no-placeholder Check. Before this budget was met,
// each of the fixture's five bracket candidates cost an extra string()
// copy in the ContainsBodyToken call regardless of which token was
// configured.
func TestCheckAllocBudgetWithPlaceholders(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	src := []byte(allocBudgetPlaceholderFixture)

	// heading-question's check (strings.TrimSpace(text) == "?") does not
	// itself allocate on a no-match input, unlike var-token's fieldinterp
	// path (a regexp scan with its own allocation cost) — isolating the
	// per-candidate string() copy this test targets from that unrelated
	// cost.
	without := checkAllocsPerOpOn(t, &Rule{}, src)
	with := checkAllocsPerOpOn(t, &Rule{Placeholders: []string{"heading-question"}}, src)
	marginal := with - without
	if marginal < 0 {
		marginal = 0
	}
	t.Logf("MDS054 Check allocs/op: without placeholders=%.0f, with=%.0f, marginal=%.0f (budget = %d)",
		without, with, marginal, allocBudgetMDS054PlaceholdersMarginal)
	require.LessOrEqualf(t, marginal, float64(allocBudgetMDS054PlaceholdersMarginal),
		"MDS054 Check allocs/op: enabling placeholders costs %.0f more allocs than the same "+
			"fixture without them (budget = %d): the per-bracket-candidate placeholder check "+
			"must not allocate a string() copy of label/text on every candidate",
		marginal, allocBudgetMDS054PlaceholdersMarginal)
}

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

// TestCheckAllocBudget pins MDS054's per-Check allocation count to
// the ≤ 10 ceiling. Skipped under -race and -short, matching the
// integration matrix.
func TestCheckAllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	r := &Rule{}
	allocs := checkAllocsPerOp(t, r)
	t.Logf("MDS054 Check allocs/op = %.0f (budget = %d)",
		allocs, allocBudgetMDS054)
	require.LessOrEqualf(t, allocs, float64(allocBudgetMDS054),
		"MDS054 Check allocs/op = %.0f, budget = %d (plan 195)",
		allocs, allocBudgetMDS054)
}
