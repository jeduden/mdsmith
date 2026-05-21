package integration

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/require"
)

// allocBudgetCeiling is the per-rule per-Check upper bound documented
// in CLAUDE.md ("A rule's Check allocates ≤ 10 times per call on
// representative input"). The plan 193 / MDS024 gate uses 9; this
// integration gate uses 10 — the published ceiling — so every other
// rule is measured against the same bar a contributor reads in the
// codebase docs.
const allocBudgetCeiling = 10

// allocBudgetGrandfathered lists rules with documented in-progress
// fixes scheduled in plan 195. Each value is the upper bound the
// rule is currently allowed to allocate; the gate fails if the rule
// exceeds *its grandfathered limit* (so a regression past today's
// baseline still trips) but does not require the rule to fit the
// ceiling until plan 195 lands its dedicated fix.
//
// New entries MUST be accompanied by a plan 195 task entry; removing
// an entry requires the rule's gate-budget unit test (e.g.
// internal/rules/<pkg>/alloc_test.go) to pass at the ≤
// allocBudgetCeiling target. The map shrinks as fixes land.
//
// Recorded baselines on the integration fixture as of the gate's
// first run. Mid-fix rules (MDS025, MDS026) carry the post-partial
// number; full-fix rules are absent.
var allocBudgetGrandfathered = map[string]int{
	// MDS025 absorbed the GFM structure checks (MD055/056/058) when
	// plan 181 folded MDS060 into it; the structure pass parses every
	// row a second time alongside tablefmt's alignment scan. Reducing
	// this to the ≤ 10 ceiling needs the single-table-walk refactor
	// scheduled as a follow-up to plan 181.
	"MDS025": 110, // table-format
	"MDS026": 18,  // table-readability
	"MDS027": 25,  // cross-file-reference-integrity
	"MDS029": 398, // conciseness-scoring
	"MDS035": 201, // toc-directive
	"MDS036": 12,  // max-section-length
	"MDS053": 16,  // no-unused-link-definitions
	"MDS054": 21,  // no-undefined-reference-labels
	"MDS063": 17,  // descriptive-link-text
	// Baselines tightened to the post-perf-chunk numbers so a
	// regression from today's state fails CI without waiting for
	// the per-rule alloc budget to be missed by a wide margin.
	// MDS023, MDS024, MDS062 dropped under the ≤ 10 ceiling after
	// the engine-bench allocator chunk (LineOfOffset inlined binary
	// search, message-string cache, slot value semantics). Removed
	// from the grandfather list per the gate's self-removal rule.
}

// allocBudgetFixture is the representative Markdown body every rule
// is measured against. It exercises a typical mix of features —
// heading, prose paragraph, fenced code, inline link, reference
// link, list, table, link-reference definition — so each rule's
// categorisation and walk paths fire. The body is compliant: every
// line stays under 80 chars and no rule's default-settings emit a
// diagnostic. The gate measures each rule's BASE per-Check cost —
// the work the rule pays just to scan a representative file — and
// not per-violation overhead, which legitimately scales with the
// number of diagnostics a rule produces.
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

// allocBudgetFS is a minimal in-memory filesystem so rules that
// short-circuit on f.FS == nil (the cross-file / directive rules)
// reach their real work, and the link target the fixture references
// resolves cleanly. ModTime is the zero Time to keep the FS map
// hash stable across runs.
func allocBudgetFS() fstest.MapFS {
	return fstest.MapFS{
		"doc.md": &fstest.MapFile{
			Data:    []byte(allocBudgetFixture),
			ModTime: time.Time{},
		},
		"other.md": &fstest.MapFile{
			Data:    []byte("# Other\n\nBody.\n"),
			ModTime: time.Time{},
		},
	}
}

// allocsForRule returns the parse-subtracted allocs/op for r.Check on
// the shared fixture. A fresh lint.File is built per iteration so
// per-File memos start cold, matching what the engine sees in
// production (one File per Check). The parse-only baseline is
// subtracted so the number reflects rule.Check + any memos it
// triggers, not the goldmark parse the engine already pays once.
func allocsForRule(tb testing.TB, r rule.Rule) float64 {
	tb.Helper()
	src := []byte(allocBudgetFixture)
	mapFS := allocBudgetFS()
	makeFile := func(name string) *lint.File {
		f, err := lint.NewFile(name, src)
		require.NoError(tb, err)
		f.FS = mapFS
		f.RootDir = "."
		f.RunCache = lint.NewRunCache()
		return f
	}
	// Warm: prime any package-level singletons (tokenizer init,
	// regex compile) the first Check would otherwise charge to the
	// measured frame.
	_ = r.Check(makeFile("warm.md"))

	const runs = 100
	parse := testing.AllocsPerRun(runs, func() {
		_ = makeFile("parse.md")
	})
	full := testing.AllocsPerRun(runs, func() {
		f := makeFile("check.md")
		_ = r.Check(f)
	})
	delta := full - parse
	if delta < 0 {
		delta = 0
	}
	return delta
}

// TestPerRuleAllocBudget enforces the CLAUDE.md ≤ 10 allocs/op
// per-Check ceiling across every registered rule. Each rule runs as
// its own subtest, so a regression names the offending rule and
// leaves the rest of the matrix visible. Skipped under -race
// (allocation bookkeeping perturbs counts) and -short (the
// AllocsPerRun loops are 100×).
func TestPerRuleAllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race; the race detector " +
			"perturbs allocation counts")
	}
	rules := rule.All()
	sort.Slice(rules, func(i, j int) bool { return rules[i].ID() < rules[j].ID() })
	for _, r := range rules {
		r := r
		t.Run(r.ID()+"_"+r.Name(), func(t *testing.T) {
			allocs := allocsForRule(t, r)
			budget := float64(allocBudgetCeiling)
			if g, ok := allocBudgetGrandfathered[r.ID()]; ok {
				budget = float64(g)
			}
			// testing.AllocsPerRun returns a float64 average across
			// many iterations; a fractional value like 10.9 must trip
			// a budget of 10, not silently round down (Copilot review
			// PR #368). Compare the float directly.
			if allocs > budget {
				if budget == float64(allocBudgetCeiling) {
					t.Fatalf("%s (%s) Check allocates %.1f/op, ceiling = %d "+
						"(CLAUDE.md ≤ 10 per call on representative input)",
						r.ID(), r.Name(), allocs, allocBudgetCeiling)
				}
				t.Fatalf("%s (%s) Check allocates %.1f/op, grandfathered "+
					"budget = %.0f. The grandfather list at the top of this "+
					"file pins the rule's plan-195 baseline so regressions "+
					"past it fail CI even before the rule is fully under "+
					"the ≤ %d ceiling. Either fix the regression or, if "+
					"the new cost is justified, raise the grandfathered "+
					"entry with a plan 195 task note.",
					r.ID(), r.Name(), allocs, budget, allocBudgetCeiling)
			}
			// When a grandfathered rule comes in under the ceiling, the
			// grandfather row is stale; surface it so the next contributor
			// removes it.
			if _, ok := allocBudgetGrandfathered[r.ID()]; ok && allocs <= float64(allocBudgetCeiling) {
				t.Fatalf("%s (%s) now allocates %.1f/op (≤ %d ceiling) — "+
					"remove the grandfather entry in allocBudgetGrandfathered.",
					r.ID(), r.Name(), allocs, allocBudgetCeiling)
			}
		})
	}
}

// BenchmarkPerRuleAllocBudget reports the same numbers as the gate
// in one table so a benchmark run lists every rule's headroom at a
// glance. Useful for spotting "close to the budget" rules that did
// not yet trip the gate but should be on the watchlist.
//
// Per-iteration work is constant (it rebuilds the entire table) and
// does not scale with b.N — so the standard Go bench harness, which
// dials b.N up to stabilise ns/op, would re-run the whole table
// many times and report meaningless ns/op. Run it explicitly with
// `-benchtime=1x` (the b.N == 1 case below) so the harness only
// invokes it once. Without the flag the function skips with a
// clear message instead of pretending to benchmark.
func BenchmarkPerRuleAllocBudget(b *testing.B) {
	if b.N != 1 {
		b.Skip("BenchmarkPerRuleAllocBudget is a per-rule allocation report, " +
			"not a microbenchmark; run with `-benchtime=1x` so it executes once")
	}
	rules := rule.All()
	sort.Slice(rules, func(i, j int) bool { return rules[i].ID() < rules[j].ID() })
	type row struct {
		id     string
		name   string
		allocs float64
	}
	rows := make([]row, 0, len(rules))
	for _, r := range rules {
		rows = append(rows, row{id: r.ID(), name: r.Name(), allocs: allocsForRule(b, r)})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].allocs > rows[j].allocs })
	var sb strings.Builder
	fmt.Fprintf(&sb, "\n%-8s %-40s %s\n", "ID", "Name", "allocs/op")
	for _, r := range rows {
		marker := " "
		if r.allocs > float64(allocBudgetCeiling) {
			marker = "!"
		}
		fmt.Fprintf(&sb, "%s %-7s %-40s %.0f\n", marker, r.id, r.name, r.allocs)
	}
	b.Log(sb.String())
}
