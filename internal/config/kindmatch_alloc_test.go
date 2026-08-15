package config

import "testing"

// TestResolveEffectiveKinds_AllocBudget pins resolveEffectiveKinds's
// per-call allocation count on a file that matches a kind-assignment
// entry. resolveEffectiveKinds only needs matchKindAssignmentEntry's
// bool verdict, but matchKindAssignmentEntry unconditionally also
// builds the provenance selector string (formatSelector) and throws
// it away — see docs/development/high-performance-go.md "Skip work
// you don't need". resolveEffectiveKinds runs on every workspace file
// via config.EffectiveSignature, so those wasted allocs pay for
// themselves once per file per matching kind-assignment entry, every
// check/fix run.
func TestResolveEffectiveKinds_AllocBudget(t *testing.T) {
	cfg := &Config{
		KindAssignment: []KindAssignmentEntry{
			{Glob: []string{"plan/*.md"}, Kinds: []string{"plan"}},
		},
	}
	allocs := testing.AllocsPerRun(200, func() {
		_ = resolveEffectiveKinds(cfg, "plan/1.md", nil, nil)
	})
	if allocs > 1 {
		t.Fatalf("resolveEffectiveKinds allocs per call: want <= 1, got %v", allocs)
	}
}
