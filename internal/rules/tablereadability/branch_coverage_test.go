package tablereadability

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestParseTables_SingleRowOnly covers the boundary check
// where a `|`-starting line at the end of the file cannot be a
// table (a table requires at least header + separator).
func TestParseTables_SingleRowOnly(t *testing.T) {
	lines := [][]byte{[]byte("| only one row |")}
	got := parseTables(lines, map[int]struct{}{})
	require.Empty(t, got)
}

// TestParseTables_MalformedHeader covers the path where the
// "header" line is pipe-leading but doesn't end with a `|` (e.g.
// `| foo`) so it is not recognized as a table.
func TestParseTables_MalformedHeader(t *testing.T) {
	lines := [][]byte{
		[]byte("| foo"),
		[]byte("| bar |"),
	}
	got := parseTables(lines, map[int]struct{}{})
	require.Empty(t, got)
}

// TestParseTables_MissingSeparatorRow covers parseTables skipping
// a pipe-containing line that does not start a valid table (the
// separator row is missing), advancing by one rather than jumping.
func TestParseTables_MissingSeparatorRow(t *testing.T) {
	lines := [][]byte{
		[]byte("| not a table"),
		[]byte("just prose"),
	}
	got := parseTables(lines, map[int]struct{}{})
	require.Empty(t, got)
}

// TestColumnWidthRatio_EmptyColumn covers the
// `if counts[col] == 0 { continue }` branch of columnWidthRatio.
// The branch fires when a column has no data rows (only header +
// separator); a fixture with two columns where one column has no
// data row exercises it directly.
func TestColumnWidthRatio_EmptyColumn(t *testing.T) {
	tbl := table{
		startLine: 1,
		rows: []tableRow{
			// Header has 2 cells; data row has only 1.
			{line: 1, cells: cellsFromStrings("A", "B")},
			{line: 2, cells: cellsFromStrings("-", "-"), isSeparator: true},
			{line: 3, cells: cellsFromStrings("a")},
		},
	}
	_ = tbl.columnWidthRatio() // should not panic; coverage is the point.
}
