package linelength

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// BenchmarkRule_MDS001 reports allocs/op for one Check call on the
// representative fixture used by TestCheckAllocBudget. Useful for
// profiling (`-cpuprofile`/`-memprofile`) when investigating a
// regression beyond what the gate's single number reveals.
func BenchmarkRule_MDS001(b *testing.B) {
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	src := []byte(allocBudgetFixture)
	warm, err := lint.NewFile("warm.md", src)
	require.NoError(b, err)
	_ = r.Check(warm)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		f, err := lint.NewFile("bench.md", src)
		require.NoError(b, err)
		_ = r.Check(f)
	}
}
