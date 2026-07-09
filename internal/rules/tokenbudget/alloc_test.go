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
