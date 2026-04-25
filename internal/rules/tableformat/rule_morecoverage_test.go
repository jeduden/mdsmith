package tableformat

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Category coverage ---

func TestCategory_TableFormat(t *testing.T) {
	r := &Rule{Pad: 1}
	assert.Equal(t, "table", r.Category())
}

// --- Fix: tables == 0 branch ---

func TestFix_NoTables_ReturnsCopy(t *testing.T) {
	// When there are no tables, Fix should return a copy of the source.
	src := "# Just a heading\n\nSome text, no tables here.\n"
	r := &Rule{Pad: 1}
	f := newTestFile(t, src)
	result := r.Fix(f)
	assert.Equal(t, src, string(result))
}

// --- Fix: tableEqual branch (already formatted table) ---

func TestFix_AlreadyFormattedTable_NoCopy(t *testing.T) {
	// Table is already properly formatted — Fix should return it unchanged.
	src := "| a   | b      |\n|-----|--------|\n| foo | barbaz |\n"
	r := &Rule{Pad: 1}
	f := newTestFile(t, src)
	result := r.Fix(f)
	assert.Equal(t, src, string(result))
}

// --- tryParseTable edge cases ---

// start >= len(lines) branch (exercise via findTables with an empty slice)
func TestTryParseTable_StartAtEnd(t *testing.T) {
	// An empty lines slice means tryParseTable is never called, but we can
	// call it directly with start == len(lines).
	lines := [][]byte{[]byte("| a | b |")}
	tbl, end := tryParseTable(lines, 1, nil) // start == len(lines)
	assert.Nil(t, tbl)
	assert.Equal(t, 1, end)
}

// start+1 >= len(lines): only one line that looks like a table row.
func TestTryParseTable_OnlyHeaderNoSeparator(t *testing.T) {
	lines := [][]byte{[]byte("| a | b |")} // only 1 line
	tbl, _ := tryParseTable(lines, 0, nil)
	assert.Nil(t, tbl, "expected nil when there's no separator line")
}

// Second line is not a table row.
func TestTryParseTable_SecondLineNotTableRow(t *testing.T) {
	lines := [][]byte{
		[]byte("| a | b |"),
		[]byte("some plain text"),
	}
	tbl, _ := tryParseTable(lines, 0, nil)
	assert.Nil(t, tbl, "expected nil when second line is not a table row")
}

// Second line is a row but not a separator.
func TestTryParseTable_SecondLineNotSeparator(t *testing.T) {
	lines := [][]byte{
		[]byte("| a | b |"),
		[]byte("| c | d |"),
	}
	tbl, _ := tryParseTable(lines, 0, nil)
	assert.Nil(t, tbl, "expected nil when second line is not a separator")
}

// Data row is inside a code block — breaks loop.
func TestTryParseTable_DataRowInCodeBlock(t *testing.T) {
	// Lines: header(1), separator(2), data-inside-code(3).
	// codeLines[3] = true means line 3 is in a code block, so data row loop breaks.
	lines := [][]byte{
		[]byte("| a | b |"),
		[]byte("|---|---|"),
		[]byte("| 1 | 2 |"),
	}
	codeLines := map[int]bool{3: true} // 1-based: line 3 is in code
	tbl, end := tryParseTable(lines, 0, codeLines)
	require.NotNil(t, tbl, "expected a valid table (header+sep only)")
	// end should be 2 (not 3), because data row at index 2 is in code
	assert.Equal(t, 2, end)
	assert.Len(t, tbl.rows, 2, "only header and separator rows expected")
}

// --- detectPrefix: naked > without trailing space (single > before another >) ---

func TestDetectPrefix_BlockquoteNakedGtBeforeGt(t *testing.T) {
	// ">>" with a space after: "> > | a |" — two blockquote levels
	// detectPrefix handles "> " prefix and recurses. The existing test
	// TestDetectPrefix_NestedBlockquote already covers that.
	// Here we test the raw ">" (no trailing space) followed by ">":
	// remaining = ">>| a |", trimmed[1]=='>' so second branch fires.
	line := []byte(">>| a | b |")
	prefix := detectPrefix(line)
	// First iteration: ">>" starts with ">", trimmed[1]=='>', consumes one ">"
	// Second iteration: ">| a | b |", trimmed[1]=='|', neither branch matches → break
	// Result: ">"
	assert.Equal(t, ">", prefix)
}

// --- formatTable: len(tbl.rows) < 2 ---

func TestFormatTable_LessThanTwoRows(t *testing.T) {
	// A table with only one row should be returned as-is.
	tbl := table{
		startLine: 1,
		rawLines:  [][]byte{[]byte("| a | b |")},
		rows:      []row{{cells: []string{"a", "b"}}},
	}
	result := formatTable(tbl, 1)
	// Should be the same object (returned as-is)
	assert.Equal(t, tbl.rawLines, result.rawLines)
}

// --- writeSeparatorRow: extend aligns to match numCols ---

func TestWriteSeparatorRow_ExtendsAlignments(t *testing.T) {
	// Separator row has fewer alignments than columns — should extend with alignNone.
	// We exercise this via a table where the separator has fewer cells than the header.
	src := "| a | b | c |\n|---|---|\n| 1 | 2 | 3 |\n"
	r := &Rule{Pad: 1}
	f := newTestFile(t, src)
	// Fix should not panic and should produce a valid table.
	result := r.Fix(f)
	assert.Contains(t, string(result), "| a   | b   | c   |")
}

// --- tableDiffMessage: i >= len(formatted.rawLines) branch ---

func TestTableDiffMessage_OriginalLonger(t *testing.T) {
	// Original has more lines than formatted — the loop should break at
	// i >= len(formatted.rawLines) and return fallback message.
	orig := table{
		rawLines: [][]byte{
			[]byte("| a |"),
			[]byte("|---|"),
			[]byte("| 1 |"),
			[]byte("| 2 |"), // extra row not in formatted
		},
	}
	formatted := table{
		rawLines: [][]byte{
			[]byte("| a |"),
			[]byte("|---|"),
			[]byte("| 1 |"),
		},
	}
	msg := tableDiffMessage(orig, formatted)
	// Falls through loop without finding a diff (i >= len(formatted.rawLines) breaks),
	// returns the fallback "table is not formatted".
	assert.Equal(t, "table is not formatted", msg)
}

// --- tableEqual: different lengths ---

func TestTableEqual_DifferentLengths(t *testing.T) {
	a := table{rawLines: [][]byte{[]byte("| a |"), []byte("|---|")}}
	b := table{rawLines: [][]byte{[]byte("| a |")}}
	assert.False(t, tableEqual(a, b))
}
