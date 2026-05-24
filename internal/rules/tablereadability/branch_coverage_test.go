package tablereadability

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTryParseTable_StartAtLastLine covers the boundary check
// where a `|`-starting line at the end of the file cannot be a
// table (a table requires at least header + separator). The
// negative arm returns nil without paying detectPrefix.
func TestTryParseTable_StartAtLastLine(t *testing.T) {
	lines := [][]byte{[]byte("| only one row |")}
	tbl, end := tryParseTable(lines, 0, map[int]bool{})
	require.Nil(t, tbl)
	require.Equal(t, 0, end)
}

// TestTryParseTable_HeaderNotTableRow covers the path where the
// "header" line is pipe-leading but doesn't end with a `|` (e.g.
// `| foo`) so isTableRow returns false.
func TestTryParseTable_HeaderNotTableRow(t *testing.T) {
	lines := [][]byte{
		[]byte("| foo"),
		[]byte("| bar |"),
	}
	tbl, _ := tryParseTable(lines, 0, map[int]bool{})
	require.Nil(t, tbl)
}

// TestFindTables_TryParseReturnsNil covers findTables' inner
// `if tbl == nil` arm when tryParseTable finds a pipe line that
// does not start a table — the loop advances by one rather than
// jumping to `end`.
func TestFindTables_TryParseReturnsNil(t *testing.T) {
	// A pipe-leading line followed by a non-pipe line — tryParseTable
	// returns nil because the separator row is missing.
	lines := [][]byte{
		[]byte("| not a table"),
		[]byte("just prose"),
	}
	got := findTables(lines, map[int]bool{})
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
