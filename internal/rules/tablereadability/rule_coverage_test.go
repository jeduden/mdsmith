package tablereadability

import (
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

// --- toFloat coverage ---

func TestToFloat_Float64(t *testing.T) {
	n, ok := toFloat(float64(3.14))
	assert.True(t, ok)
	assert.InDelta(t, 3.14, n, 0.001)
}

func TestToFloat_Int(t *testing.T) {
	n, ok := toFloat(42)
	assert.True(t, ok)
	assert.Equal(t, float64(42), n)
}

func TestToFloat_Int64(t *testing.T) {
	n, ok := toFloat(int64(7))
	assert.True(t, ok)
	assert.Equal(t, float64(7), n)
}

func TestToFloat_String_Fails(t *testing.T) {
	_, ok := toFloat("not a number")
	assert.False(t, ok)
}

func TestToFloat_Bool_Fails(t *testing.T) {
	_, ok := toFloat(true)
	assert.False(t, ok)
}

func TestToFloat_Nil_Fails(t *testing.T) {
	_, ok := toFloat(nil)
	assert.False(t, ok)
}

// --- toInt coverage ---

func TestToInt_Int(t *testing.T) {
	n, ok := toInt(42)
	assert.True(t, ok)
	assert.Equal(t, 42, n)
}

func TestToInt_Float64(t *testing.T) {
	n, ok := toInt(float64(3.0))
	assert.True(t, ok)
	assert.Equal(t, 3, n)
}

func TestToInt_Int64(t *testing.T) {
	n, ok := toInt(int64(7))
	assert.True(t, ok)
	assert.Equal(t, 7, n)
}

func TestToInt_String_Fails(t *testing.T) {
	_, ok := toInt("not a number")
	assert.False(t, ok)
}

func TestToInt_Nil_Fails(t *testing.T) {
	_, ok := toInt(nil)
	assert.False(t, ok)
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
