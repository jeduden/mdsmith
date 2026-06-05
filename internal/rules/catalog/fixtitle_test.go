package catalog

import "testing"

// TestFixTitle exercises the rule's QuickFixTitler label so the
// editor quick-fix presents a specific action. Asserted non-empty
// here; internal/rules/all enforces it is more specific than the
// generic "Fix all <name>" fallback across the whole rule set.
func TestFixTitle(t *testing.T) {
	if got := (&Rule{}).FixTitle(); got == "" {
		t.Error("FixTitle returned empty string")
	}
}
