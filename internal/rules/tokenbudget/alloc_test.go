package tokenbudget

import (
	"strings"
	"testing"
)

// TestCheck_AllocBudget_OverBudget pins Check's per-call allocation
// count on a violating file whose count/budget both exceed 99 — the
// realistic case for token budgets, which run in the thousands.
// BenchmarkCheck_OverBudget shows the strconv-based message build
// (replacing fmt.Sprintf) is ~19% faster in CPU time and 9% smaller
// in bytes/op (benchstat, p=0.000) despite allocs/op staying flat at
// 6. See docs/development/high-performance-go.md "strconv over
// fmt.Sprintf".
func TestCheck_AllocBudget_OverBudget(t *testing.T) {
	src := []byte(strings.Repeat("word ", 200))
	f := mustFile(t, "test.md", string(src))
	r := &Rule{Max: 100, Mode: "heuristic", TokensPerWord: 1.0}
	allocs := testing.AllocsPerRun(200, func() {
		_ = r.Check(f)
	})
	if allocs > 6 {
		t.Fatalf("Check allocs per call: want <= 6, got %v", allocs)
	}
}

// TestCheck_AllocBudget_UnderBudget pins Check's per-call allocation
// count for the common case docs/development/high-performance-go.md's
// "cheap pre-check" pattern targets: a small file under the default
// 8000-token budget. definitelyUnderBudget's len(source)-only check
// lets Check return before mdtext.CountWordsBytes ever scans the
// file's words, so this stays at 0 allocs/op regardless of file size
// (as long as it is under budget) — unlike TestCheck_AllocBudget_OverBudget,
// which must run the real scan and therefore allocates.
func TestCheck_AllocBudget_UnderBudget(t *testing.T) {
	src := []byte(strings.Repeat("some prose word ", 200))
	f := mustFile(t, "test.md", string(src))
	r := &Rule{Max: defaultMax, Mode: "heuristic", TokensPerWord: defaultTokensPerWord}
	allocs := testing.AllocsPerRun(200, func() {
		_ = r.Check(f)
	})
	if allocs > 0 {
		t.Fatalf("Check allocs per call for an under-budget file: want 0, got %v", allocs)
	}
}
