package tablereadability

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Note: this file uses newFile() defined in rule_test.go (same package).

// --- stripPrefix coverage ---

func TestStripPrefix_Empty(t *testing.T) {
	line := []byte("| a | b |")
	result := stripPrefix(line, "")
	assert.Equal(t, "| a | b |", string(result))
}

func TestStripPrefix_Match(t *testing.T) {
	line := []byte("> | a | b |")
	result := stripPrefix(line, "> ")
	assert.Equal(t, "| a | b |", string(result))
}

func TestStripPrefix_NoMatch(t *testing.T) {
	line := []byte("| a | b |")
	result := stripPrefix(line, "> ")
	assert.Equal(t, "| a | b |", string(result))
}

// --- detectPrefix coverage ---

func TestDetectPrefix_NoPrefix(t *testing.T) {
	line := []byte("| a | b |")
	prefix := detectPrefix(line)
	assert.Equal(t, "", prefix)
}

func TestDetectPrefix_Blockquote(t *testing.T) {
	line := []byte("> | a | b |")
	prefix := detectPrefix(line)
	assert.Equal(t, "> ", prefix)
}

func TestDetectPrefix_NestedBlockquote(t *testing.T) {
	line := []byte("> > | a | b |")
	prefix := detectPrefix(line)
	assert.Equal(t, "> > ", prefix)
}

func TestDetectPrefix_SpaceIndent(t *testing.T) {
	line := []byte("  | a | b |")
	prefix := detectPrefix(line)
	assert.Equal(t, "  ", prefix)
}

func TestDetectPrefix_NoPipe(t *testing.T) {
	line := []byte("text without pipe")
	prefix := detectPrefix(line)
	assert.Equal(t, "", prefix)
}

func TestDetectPrefix_CompactBlockquote(t *testing.T) {
	// ">>" followed by "> " continues stripping
	line := []byte(">> > | a | b |")
	prefix := detectPrefix(line)
	assert.Equal(t, ">> > ", prefix)
}

func TestDetectPrefix_SingleCompactGt(t *testing.T) {
	// ">" without space followed by "|" stops after first ">"
	line := []byte(">>| a | b |")
	prefix := detectPrefix(line)
	assert.Equal(t, ">", prefix)
}

func TestDetectPrefix_NonSpacePrefixBeforePipe(t *testing.T) {
	// Prefix before pipe contains non-space characters
	line := []byte("abc| d |")
	prefix := detectPrefix(line)
	assert.Equal(t, "", prefix)
}

// --- columnHeader coverage ---

func TestColumnHeader_Valid(t *testing.T) {
	tbl := table{
		rows: []tableRow{
			{cells: []string{"Name", "Value"}},
			{cells: []string{"---", "---"}, isSeparator: true},
		},
	}
	assert.Equal(t, "Name", tbl.columnHeader(0))
	assert.Equal(t, "Value", tbl.columnHeader(1))
}

func TestColumnHeader_OutOfRange(t *testing.T) {
	tbl := table{
		rows: []tableRow{
			{cells: []string{"Name"}},
		},
	}
	assert.Equal(t, "", tbl.columnHeader(5))
}

func TestColumnHeader_EmptyRows(t *testing.T) {
	tbl := table{rows: []tableRow{}}
	assert.Equal(t, "", tbl.columnHeader(0))
}

// --- isSeparatorRow coverage ---

func TestIsSeparatorRow_Valid(t *testing.T) {
	assert.True(t, isSeparatorRow([]string{"---", "---"}))
	assert.True(t, isSeparatorRow([]string{":---", "---:"}))
	assert.True(t, isSeparatorRow([]string{":---:"}))
}

func TestIsSeparatorRow_Empty(t *testing.T) {
	assert.False(t, isSeparatorRow([]string{}))
}

func TestIsSeparatorRow_Invalid(t *testing.T) {
	assert.False(t, isSeparatorRow([]string{"abc"}))
	assert.False(t, isSeparatorRow([]string{"---", "abc"}))
}

// --- ApplySettings coverage for individual fields ---

func TestApplySettings_MaxColumnsInvalid(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max-columns": "a"})
	require.Error(t, err)
}

func TestApplySettings_MaxColumnsZero(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max-columns": 0})
	require.Error(t, err)
}

func TestApplySettings_MaxColumnsNegative(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max-columns": -1})
	require.Error(t, err)
}

func TestApplySettings_MaxRowsInvalid(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max-rows": "a"})
	require.Error(t, err)
}

func TestApplySettings_MaxRowsZero(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max-rows": 0})
	require.Error(t, err)
}

func TestApplySettings_MaxWordsPerCellInvalid(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max-words-per-cell": "a"})
	require.Error(t, err)
}

func TestApplySettings_MaxWordsPerCellZero(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max-words-per-cell": 0})
	require.Error(t, err)
}

func TestApplySettings_MaxColumnWidthRatioInvalid(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max-column-width-ratio": "a"})
	require.Error(t, err)
}

func TestApplySettings_MaxColumnWidthRatioZero(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max-column-width-ratio": 0.0})
	require.Error(t, err)
}

func TestApplySettings_MaxColumnWidthRatioNeg(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max-column-width-ratio": -1.0})
	require.Error(t, err)
}

func TestApplySettings_Float64MaxColumns(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max-columns": float64(5)})
	require.NoError(t, err)
	assert.Equal(t, 5, r.MaxColumns)
}

func TestApplySettings_Int64MaxRows(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max-rows": int64(10)})
	require.NoError(t, err)
	assert.Equal(t, 10, r.MaxRows)
}

func TestApplySettings_IntMaxColumnWidthRatio(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max-column-width-ratio": 5})
	require.NoError(t, err)
	assert.Equal(t, float64(5), r.MaxColumnWidthRatio)
}

// --- Check with defaults (zero fields use fallbacks) ---

func TestCheck_ZeroFields_DefaultsUsed(t *testing.T) {
	r := &Rule{} // all zeros
	src := "# Title\n\n| A | B |\n|---|---|\n| 1 | 2 |\n"
	diags := r.Check(newFile(t, src))
	// With defaults, this small table should pass
	assert.Len(t, diags, 0)
}

// --- columnWidthRatio with single column ---

func TestColumnWidthRatio_SingleColumn(t *testing.T) {
	tbl := table{
		rows: []tableRow{
			{cells: []string{"Header"}},
			{cells: []string{"---"}, isSeparator: true},
			{cells: []string{"value"}},
		},
	}
	ratio := tbl.columnWidthRatio()
	// Single column: min==max, ratio should be 1.0
	assert.InDelta(t, 1.0, ratio, 0.001)
}

// --- Check TooManyWordsPerCell without header name ---

func TestCheck_TooManyWordsPerCell_NoHeaderColumn(t *testing.T) {
	// Column index exceeds header cells
	r := &Rule{MaxColumns: 6, MaxRows: 20, MaxWordsPerCell: 2, MaxColumnWidthRatio: 100}
	src := "# Title\n\n| A |\n|---|\n| one two three |\n"
	diags := r.Check(newFile(t, src))
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "table cell has too many words")
}

// =====================================================================
// Phase 5: additional branch coverage
// =====================================================================

// tryParseTable: separator line is not a table row → return nil
func TestTryParseTable_SeparatorNotTableRow(t *testing.T) {
	lines := [][]byte{
		[]byte("| A | B |"),
		[]byte("just text"), // not a table row
		[]byte("| C | D |"),
	}
	tbl, _ := tryParseTable(lines, 0, map[int]bool{})
	assert.Nil(t, tbl, "expected nil when separator is not a table row")
}

// tryParseTable: separator is a table row but not a separator row → return nil
func TestTryParseTable_SeparatorNotSeparatorRow(t *testing.T) {
	lines := [][]byte{
		[]byte("| A | B |"),
		[]byte("| C | D |"), // is table row but not separator
		[]byte("| E | F |"),
	}
	tbl, _ := tryParseTable(lines, 0, map[int]bool{})
	assert.Nil(t, tbl, "expected nil when second line is not a separator row")
}

// tryParseTable: separator line is in a code block → return nil
// codeLines uses 1-based line numbers; start=0, separator is at index 1 → 1-based=2
func TestTryParseTable_SeparatorInCodeBlock(t *testing.T) {
	codeLines := map[int]bool{2: true} // 1-based line 2 = index 1 (separator)
	lines := [][]byte{
		[]byte("| A | B |"),
		[]byte("|---|---|"),
		[]byte("| C | D |"),
	}
	tbl, _ := tryParseTable(lines, 0, codeLines)
	assert.Nil(t, tbl, "expected nil when separator line is a code line")
}

// tryParseTable: subsequent data row is in a code block → loop breaks early.
// With start=0, end starts at 2. codeLines[end+1]=codeLines[3] means index 2 (lines[2]).
func TestTryParseTable_DataRowInCodeBlock(t *testing.T) {
	codeLines := map[int]bool{3: true} // 1-based line 3 = index 2 (lines[2])
	lines := [][]byte{
		[]byte("| A | B |"),
		[]byte("|---|---|"),
		[]byte("| C | D |"), // this is in a code block
		[]byte("| E | F |"),
	}
	tbl, end := tryParseTable(lines, 0, codeLines)
	require.NotNil(t, tbl)
	// The loop should break when it hits lines[2] (codeLines[3]=true).
	// end should remain at 2 (start+2 initial value, loop doesn't advance).
	assert.Equal(t, 2, end, "expected end at 2 (before code-blocked line)")
}

// columnWidthRatio: columns == 0 → return 0
func TestColumnWidthRatio_NoColumns(t *testing.T) {
	tbl := table{rows: []tableRow{}}
	assert.Equal(t, 0.0, tbl.columnWidthRatio())
}

// columnWidthRatio: one column has all empty cells (avg=0) while another has content
// → minAverage == 0, maxAverage > 0 → return Inf(1)
func TestColumnWidthRatio_OneEmptyOneNonEmpty(t *testing.T) {
	// 2-column table: col0="ABC" (avg=3), col1="" (avg=0)
	// minAverage=0, maxAverage=3 → hits `if minAverage == 0 { return math.Inf(1) }`
	tbl := table{
		rows: []tableRow{
			{cells: []string{"ABC", ""}},
			{cells: []string{"---", "---"}, isSeparator: true},
			{cells: []string{"DEF", ""}},
		},
	}
	ratio := tbl.columnWidthRatio()
	assert.True(t, math.IsInf(ratio, 1), "expected +Inf when min column avg is 0")
}

// columnWidthRatio: only separator rows → columns==0 → return 0
func TestColumnWidthRatio_OnlySeparatorRows(t *testing.T) {
	tbl := table{
		rows: []tableRow{
			{cells: []string{"---"}, isSeparator: true},
		},
	}
	ratio := tbl.columnWidthRatio()
	assert.Equal(t, 0.0, ratio, "table with only separator should have ratio 0")
}

// columnWidthRatio: all data cells empty (avg=0 for all cols) →
// maxAverage == 0 → `if minAverage == math.MaxFloat64 || maxAverage == 0` fires → return 0
func TestColumnWidthRatio_AllDataCellsEmpty(t *testing.T) {
	tbl := table{
		rows: []tableRow{
			{cells: []string{"", ""}}, // header (non-sep), all empty
			{cells: []string{"---", "---"}, isSeparator: true},
		},
	}
	ratio := tbl.columnWidthRatio()
	assert.Equal(t, 0.0, ratio, "all-empty data cells should return ratio 0")
}
