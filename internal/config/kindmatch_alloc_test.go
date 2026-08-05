package config

import (
	"fmt"
	"testing"
)

// manyKindAssignmentEntries builds n kind-assignment entries that all
// match "docs/guide.md", so resolveEffectiveKinds drives every entry
// through matchKindAssignmentEntry's matched path — the one that builds
// the discarded formatSelector string.
func manyKindAssignmentEntries(n int) []KindAssignmentEntry {
	entries := make([]KindAssignmentEntry, n)
	for i := range entries {
		entries[i] = KindAssignmentEntry{
			Glob:  []string{"**/*.md"},
			Kinds: []string{fmt.Sprintf("kind%d", i)},
		}
	}
	return entries
}

// TestResolveEffectiveKindsAllocBudget pins the allocation cost of
// resolveEffectiveKinds over many kind-assignment entries.
// matchKindAssignmentEntry unconditionally built and returned a
// provenance string (formatSelector) even for its hottest caller,
// resolveEffectiveKinds, which immediately discards it
// (`matched, _ := matchKindAssignmentEntry(...)`). resolveEffectiveKinds
// runs on every file via EffectiveSignature, which the engine calls
// unconditionally to build its per-file memo key — even on a config
// cache hit — so every entry's wasted formatSelector call multiplies
// across the whole workspace
// (docs/development/high-performance-go.md "Skip work you don't need").
func TestResolveEffectiveKindsAllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race; the race detector " +
			"adds allocation bookkeeping that perturbs the count")
	}
	const numEntries = 30
	cfg := &Config{KindAssignment: manyKindAssignmentEntries(numEntries)}

	got := resolveEffectiveKinds(cfg, "docs/guide.md", nil, nil)
	if len(got) != numEntries {
		t.Fatalf("expected %d kind-assignment matches, got %d: %v", numEntries, len(got), got)
	}

	const runs = 200
	allocs := testing.AllocsPerRun(runs, func() {
		_ = resolveEffectiveKinds(cfg, "docs/guide.md", nil, nil)
	})
	const budget = 16
	t.Logf("resolveEffectiveKinds allocs/op over %d entries = %.1f (budget = %d)",
		numEntries, allocs, budget)
	if allocs > float64(budget) {
		t.Fatalf("resolveEffectiveKinds allocs/op = %.1f over %d entries, budget = %d; "+
			"matchKindAssignmentEntry should not build formatSelector for a caller "+
			"that discards it (see docs/development/high-performance-go.md)",
			allocs, numEntries, budget)
	}
}
