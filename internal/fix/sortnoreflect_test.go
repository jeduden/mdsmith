package fix

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
)

// These pin the allocation cost of four sort.Slice call sites in this
// package, which drive reflect.Swapper internally — the "reflect in
// hot paths" anti-pattern in docs/development/high-performance-go.md,
// already fixed the same way elsewhere in this codebase
// (astutil.sortSectionHeadings, mdsmith.sortDirEntries). Each sorts a
// run-scoped result list once per `mdsmith fix` invocation (or once
// per cache-key build for the rule lists), not per workspace file, so
// the win is small but free.

// fakeFixableRule is a minimal rule.FixableRule stub for exercising
// sortFixableRulesByID without pulling in a real rule package.
type fakeFixableRule struct {
	id string
}

func (r fakeFixableRule) ID() string                           { return r.id }
func (r fakeFixableRule) Name() string                         { return r.id }
func (r fakeFixableRule) Category() string                     { return "test" }
func (r fakeFixableRule) Check(f *lint.File) []lint.Diagnostic { return nil }
func (r fakeFixableRule) Fix(f *lint.File) []byte              { return nil }

func TestSortFixableRulesByID_NoReflectSort(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	fixable := []rule.FixableRule{
		fakeFixableRule{id: "MDS009"},
		fakeFixableRule{id: "MDS001"},
		fakeFixableRule{id: "MDS005"},
	}
	sortFixableRulesByID(fixable)
	if fixable[0].ID() != "MDS001" || fixable[2].ID() != "MDS009" {
		t.Fatalf("sortFixableRulesByID did not sort: %v", fixable)
	}

	const runs = 200
	allocs := testing.AllocsPerRun(runs, func() {
		sortFixableRulesByID(fixable)
	})
	t.Logf("sortFixableRulesByID allocs/op = %.0f", allocs)
	if allocs > 0 {
		t.Fatalf("sortFixableRulesByID allocs/op = %.0f, want 0 (no reflection)", allocs)
	}
}

func TestSortDiagnostics_NoReflectSort(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	diags := []lint.Diagnostic{
		{File: "z.md", Line: 1, Column: 1},
		{File: "a.md", Line: 5, Column: 1},
		{File: "a.md", Line: 2, Column: 3},
	}
	sortDiagnostics(diags)
	if diags[0].File != "a.md" || diags[0].Line != 2 || diags[2].File != "z.md" {
		t.Fatalf("sortDiagnostics did not sort: %v", diags)
	}

	const runs = 200
	allocs := testing.AllocsPerRun(runs, func() {
		sortDiagnostics(diags)
	})
	t.Logf("sortDiagnostics allocs/op = %.0f", allocs)
	if allocs > 0 {
		t.Fatalf("sortDiagnostics allocs/op = %.0f, want 0 (no reflection)", allocs)
	}
}

func TestSortRuleFixCounts_NoReflectSort(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	rules := []RuleFixCount{
		{RuleID: "MDS009", Count: 1},
		{RuleID: "MDS001", Count: 2},
	}
	sortRuleFixCounts(rules)
	if rules[0].RuleID != "MDS001" {
		t.Fatalf("sortRuleFixCounts did not sort: %v", rules)
	}

	const runs = 200
	allocs := testing.AllocsPerRun(runs, func() {
		sortRuleFixCounts(rules)
	})
	t.Logf("sortRuleFixCounts allocs/op = %.0f", allocs)
	if allocs > 0 {
		t.Fatalf("sortRuleFixCounts allocs/op = %.0f, want 0 (no reflection)", allocs)
	}
}
