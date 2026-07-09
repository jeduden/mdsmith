package maxfilelength

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// TestCheck_AllocBudget_OverLimit pins Check's per-call allocation
// count on a violating file. BenchmarkCheck_OverLimit shows the
// strconv-based message build (replacing fmt.Sprintf) is ~21% faster
// in CPU time and 4% smaller in bytes/op (benchstat, p=0.000) despite
// allocs/op staying flat at 4 — see docs/development/high-performance-go.md
// "strconv over fmt.Sprintf".
func TestCheck_AllocBudget_OverLimit(t *testing.T) {
	src := []byte(nLines(301))
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Max: 300}
	allocs := testing.AllocsPerRun(200, func() {
		_ = r.Check(f)
	})
	if allocs > 4 {
		t.Fatalf("Check allocs per call: want <= 4, got %v", allocs)
	}
}
