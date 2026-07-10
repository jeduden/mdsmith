package firstlineheading

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
)

// TestCheck_WrongLevel_MessageAllocs pins the wrong-level violation
// diagnostic's per-call allocation count (≤2: 1 msg + 1 slice). A
// prior session measured strconv.Itoa+concat at 4 allocs here and
// reverted to fmt.Sprintf; re-measured on the current Go toolchain,
// strconv.Itoa caches formatted results for 0-99 (heading levels are
// always 1-6), so both approaches cost the same 2 allocs — strconv's
// win is CPU only (skips fmt's reflection), confirmed via
// BenchmarkCheck_WrongLevel + benchstat. See
// docs/development/high-performance-go.md "strconv over fmt.Sprintf".
// Uses the nil-AST (Layer 0) path to isolate message construction allocs.
func TestCheck_WrongLevel_MessageAllocs(t *testing.T) {
	src := []byte("## Not H1\n\nText\n")
	f := lint.NewFileLines("f.md", src)
	r := &Rule{Level: 1}
	_ = r.Check(f) // warm up Layer0 cache
	allocs := testing.AllocsPerRun(50, func() { _ = r.Check(f) })
	if allocs > 2 {
		t.Fatalf("Check (nil-AST wrong level): got %g allocs/call, want ≤ 2 (1 msg + 1 slice)", allocs)
	}
}
