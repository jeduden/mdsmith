package noinlinehtml

import "testing"

// allocBudgetExtractTagUppercase is the per-call ceiling for
// extractTag on a mixed-case tag name. tagNameRe.FindSubmatch pays
// one allocation for the match slice; the old
// strings.ToLower(string(m[1])) paid two more whenever m[1] contained
// an uppercase byte — the string(m[1]) copy, then strings.ToLower's
// own buffer since it cannot return the input unchanged.
// asciiLowerTag folds those two into one allocation, bringing the
// total from 3 down to 2 — see
// docs/development/high-performance-go.md "Allocations."
const allocBudgetExtractTagUppercase = 2

func TestExtractTagUppercaseAllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	raw := []byte("<DIV class=\"x\">")
	_ = extractTag(raw) // warm up regex program cache
	allocs := testing.AllocsPerRun(200, func() {
		_ = extractTag(raw)
	})
	t.Logf("extractTag(uppercase) allocs/op = %.0f (budget = %d)", allocs, allocBudgetExtractTagUppercase)
	if allocs > float64(allocBudgetExtractTagUppercase) {
		t.Fatalf("extractTag(uppercase) allocs/op = %.0f, budget = %d: lowercase the "+
			"matched bytes directly instead of string(m[1]) followed by strings.ToLower",
			allocs, allocBudgetExtractTagUppercase)
	}
}
