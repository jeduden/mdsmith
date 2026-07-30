package checker

import (
	"fmt"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
)

// perfStubRule is a plain rule.Rule (not a NodeChecker/BlockChecker) whose
// Check always returns exactly one diagnostic, so a slice of N of these
// forces CheckConfiguredRules's diags-collection loop through N single-
// diagnostic appends — the repeated-grow case the pre-sizing fix targets.
type perfStubRule struct {
	id string
}

func (r *perfStubRule) ID() string       { return r.id }
func (r *perfStubRule) Name() string     { return r.id }
func (r *perfStubRule) Category() string { return "test" }
func (r *perfStubRule) Check(_ *lint.File) []lint.Diagnostic {
	return []lint.Diagnostic{{Line: 1, RuleID: r.id, Message: "hit"}}
}

var _ rule.Rule = (*perfStubRule)(nil)

// buildPerfRules returns n configured stub rules and the matching
// effective-config map, all enabled.
func buildPerfRules(n int) ([]rule.Rule, map[string]config.RuleCfg) {
	rules := make([]rule.Rule, 0, n)
	eff := make(map[string]config.RuleCfg, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("TST%03d", i)
		rules = append(rules, &perfStubRule{id: id})
		eff[id] = config.RuleCfg{Enabled: true}
	}
	return rules, eff
}

// TestCheckConfiguredRules_DiagCollectionAllocs pins the allocation cost
// of CheckConfiguredRules's diagnostic-collection step. Before pre-sizing
// `diags`, collecting 25 rules' worth of one-diagnostic slots forces
// `append` through several grow-and-copy steps beyond what 2 rules need;
// after pre-sizing, the per-call allocs stay flat regardless of rule
// count since the backing array is sized exactly once. The ceiling below
// is chosen to sit between the unfixed 25-rule number and the fixed one.
func TestCheckConfiguredRules_DiagCollectionAllocs(t *testing.T) {
	const smallN = 2
	const bigN = 25

	smallRules, smallEff := buildPerfRules(smallN)
	smallConfigured, errs := ConfigureEnabledRules(smallRules, smallEff)
	if len(errs) != 0 {
		t.Fatalf("unexpected configure errors: %v", errs)
	}

	bigRules, bigEff := buildPerfRules(bigN)
	bigConfigured, errs := ConfigureEnabledRules(bigRules, bigEff)
	if len(errs) != 0 {
		t.Fatalf("unexpected configure errors: %v", errs)
	}

	newFile := func() *lint.File {
		f, err := lint.NewFile("doc.md", []byte("# Hello\n\nParagraph.\n"))
		if err != nil {
			t.Fatalf("NewFile: %v", err)
		}
		f.RootDir = "."
		f.RunCache = lint.NewRunCache()
		return f
	}

	smallAllocs := testing.AllocsPerRun(50, func() {
		f := newFile()
		_ = CheckConfiguredRules(f, smallConfigured, true, 1)
	})

	bigAllocs := testing.AllocsPerRun(50, func() {
		f := newFile()
		_ = CheckConfiguredRules(f, bigConfigured, true, 1)
	})

	t.Logf("smallN=%d allocs/op=%.1f bigN=%d allocs/op=%.1f", smallN, smallAllocs, bigN, bigAllocs)

	// With diags pre-sized in one pass, the extra 23 diagnostic-bearing
	// rules add only their own per-slot allocations (one Diagnostic slice
	// per rule from Check itself) with no additional grow-and-copy
	// allocations from the collection loop. The delta between big and
	// small should stay close to (bigN - smallN), not blow past it the
	// way repeated append growth would.
	delta := bigAllocs - smallAllocs
	const maxDelta = float64(bigN-smallN) + 3
	if delta > maxDelta {
		t.Fatalf("allocs/op delta too high: smallN=%.1f bigN=%.1f delta=%.1f, "+
			"want <= %.1f (unbounded append growth in the diags-collection loop)",
			smallAllocs, bigAllocs, delta, maxDelta)
	}
}
