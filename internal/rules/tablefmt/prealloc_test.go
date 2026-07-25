package tablefmt

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// wideTableLines builds a table with n data rows, used to make append
// slice-growth in tryParseTable/findTables visible in an alloc count.
func wideTableLines(n int) [][]byte {
	lines := [][]byte{
		[]byte("| Col A | Col B |"),
		[]byte("| --- | --- |"),
	}
	for i := 0; i < n; i++ {
		lines = append(lines, []byte("| a | b |"))
	}
	return lines
}

// TestFindTables_PreSizedRowSlices pins the allocation count for parsing
// one large table. rawLines and rows in tryParseTable used to grow via
// plain append with no capacity hint, paying several slice-growth
// reallocs per table (docs/development/high-performance-go.md); a
// pre-sizing pass ahead of the capture loop removes them.
func TestFindTables_PreSizedRowSlices(t *testing.T) {
	lines := wideTableLines(50)
	codeLines := map[int]struct{}{}

	allocs := testing.AllocsPerRun(50, func() {
		tables := findTables(lines, codeLines)
		require.Len(t, tables, 1)
	})
	t.Logf("findTables allocs/op for a 52-row table = %.0f", allocs)
	// Most of the remaining allocs come from splitRowBytes's per-row
	// cells slice, unrelated to this fix. Before pre-sizing
	// rawLines/rows in tryParseTable, this fixture measured 174
	// allocs/op; pre-sizing removed the slice-growth reallocs on both
	// slices, dropping it to 162.
	require.LessOrEqualf(t, allocs, float64(162),
		"findTables allocs/op = %.0f; want <= 162 (rawLines+rows pre-sized "+
			"in tryParseTable)", allocs)
}
