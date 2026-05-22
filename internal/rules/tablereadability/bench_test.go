package tablereadability

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

func BenchmarkRule_MDS026(b *testing.B) {
	r := &Rule{
		MaxColumns: defaultMaxColumns, MaxRows: defaultMaxRows,
		MaxWordsPerCell: defaultMaxWordsPerCell, MaxColumnWidthRatio: defaultMaxColumnWidthRatio,
	}
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
