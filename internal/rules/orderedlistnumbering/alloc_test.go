package orderedlistnumbering

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// TestCheckItem_AllocBudget_NoRegression pins checkItem's per-call
// allocation count. strconv.Itoa + concatenation costs the same 2
// allocs/op as the fmt.Sprintf it replaced on this input (one for the
// multi-digit Itoa(300), one for the final concat — Go's strconv caches
// single-digit results so those are free either way), but BenchmarkCheckItem
// shows it is ~42% faster in CPU time and 9% smaller in bytes/op
// (benchstat, p=0.000), since it skips fmt's reflection-driven formatting
// path. See docs/development/high-performance-go.md "strconv over
// fmt.Sprintf".
func TestCheckItem_AllocBudget_NoRegression(t *testing.T) {
	src := []byte("300. a\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: StyleSequential, Start: 1}
	allocs := testing.AllocsPerRun(200, func() {
		_, _ = r.checkItem(f, 1, 0)
	})
	if allocs > 2 {
		t.Fatalf("checkItem allocs per call: want <= 2, got %v", allocs)
	}
}
