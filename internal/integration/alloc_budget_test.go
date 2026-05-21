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

// allocBudgetFixture is the representative Markdown body every rule
// is measured against. It mirrors the engine bench's buildCorpusDoc
// — heading, prose, fenced code, link — and adds a small table, an
// emphasis run, a list, a reference link, and a heading-2 so the
// table, list-style, link-graph, structural, and heading-walk rules
// each have something to exercise. The body stays paragraph-sized
// (real-Markdown representative, not an artificially long join) to
// match what one Check call sees in production.
const allocBudgetFixture = "# Document title\n" +
	"\n" +
	"This is a representative sentence used to exercise the prose, " +
	"heading, and structural rules under the alloc budget gate. " +
	"It is intentionally one paragraph long.\n" +
	"\n" +
	"## Section\n" +
	"\n" +
	"See [the other doc](other.md) for details, " +
	"and [a label][ref] for a reference link.\n" +
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
			if allocs > float64(allocBudgetCeiling) {
				t.Fatalf("%s (%s) Check allocates %.0f/op, ceiling = %d "+
					"(CLAUDE.md ≤ 10 per call on representative input)",
					r.ID(), r.Name(), allocs, allocBudgetCeiling)
			}
		})
	}
}

// BenchmarkPerRuleAllocBudget reports the same numbers as the gate
// in one table so a benchmark run lists every rule's headroom at a
// glance. Useful for spotting "close to the budget" rules that did
// not yet trip the gate but should be on the watchlist.
func BenchmarkPerRuleAllocBudget(b *testing.B) {
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
