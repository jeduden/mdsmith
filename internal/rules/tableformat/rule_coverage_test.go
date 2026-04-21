package tableformat

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- GetPad coverage ---

func TestGetPad(t *testing.T) {
	r := &Rule{Pad: 1}
	assert.Equal(t, 1, r.GetPad())
}

func TestGetPad_Zero(t *testing.T) {
	r := &Rule{Pad: 0}
	assert.Equal(t, 0, r.GetPad())
}

func TestGetPad_Custom(t *testing.T) {
	r := &Rule{Pad: 3}
	assert.Equal(t, 3, r.GetPad())
}

// --- FormatString coverage ---

func TestFormatString_FormatsTable(t *testing.T) {
	src := "| a | b |\n|---|---|\n| foo | barbaz |\n"
	result := FormatString(src, 1)
	assert.Contains(t, result, "| a   | b      |")
	assert.Contains(t, result, "| foo | barbaz |")
}

func TestFormatString_NoTable(t *testing.T) {
	src := "# Heading\n\nSome text.\n"
	result := FormatString(src, 1)
	assert.Equal(t, src, result)
}

func TestFormatString_AlreadyFormatted(t *testing.T) {
	src := "| a   | b      |\n|-----|--------|\n| foo | barbaz |\n"
	result := FormatString(src, 1)
	assert.Equal(t, src, result)
}

func TestFormatString_MultipleTables(t *testing.T) {
	src := "| a | b |\n|---|---|\n| 1 | 2 |\n\n| x | y |\n|---|---|\n| 3 | 4 |\n"
	result := FormatString(src, 1)
	// Both tables should be formatted
	assert.Contains(t, result, "| a   | b   |")
	assert.Contains(t, result, "| x   | y   |")
}

func TestFormatString_Pad0(t *testing.T) {
	src := "| a | b |\n|---|---|\n| 1 | 2 |\n"
	result := FormatString(src, 0)
	// With pad=0, no spaces around cell content
	assert.Contains(t, result, "|a  |b  |")
}

// --- tableDiffMessage coverage ---

func TestTableDiffMessage_FirstRowDiffers(t *testing.T) {
	orig := table{
		rawLines: [][]byte{[]byte("| a | b |"), []byte("|---|---|"), []byte("| 1 | 2 |")},
	}
	formatted := table{
		rawLines: [][]byte{[]byte("| a   | b   |"), []byte("|-----|-----|"), []byte("| 1   | 2   |")},
	}
	msg := tableDiffMessage(orig, formatted)
	assert.Contains(t, msg, "row 1")
	assert.Contains(t, msg, "| a   | b   |")
}

func TestTableDiffMessage_SecondRowDiffers(t *testing.T) {
	orig := table{
		rawLines: [][]byte{[]byte("| a   | b   |"), []byte("|---|---|"), []byte("| 1 | 2 |")},
	}
	formatted := table{
		rawLines: [][]byte{[]byte("| a   | b   |"), []byte("|-----|-----|"), []byte("| 1   | 2   |")},
	}
	msg := tableDiffMessage(orig, formatted)
	assert.Contains(t, msg, "row 2")
}

func TestTableDiffMessage_AllSame(t *testing.T) {
	tbl := table{
		rawLines: [][]byte{[]byte("| a |"), []byte("|---|"), []byte("| 1 |")},
	}
	msg := tableDiffMessage(tbl, tbl)
	assert.Equal(t, "table is not formatted", msg)
}

// --- ApplySettings with float64 pad ---

func TestApplySettings_Float64Pad(t *testing.T) {
	r := &Rule{Pad: 1}
	err := r.ApplySettings(map[string]any{"pad": float64(2)})
	require.NoError(t, err)
	assert.Equal(t, 2, r.Pad)
}

func TestApplySettings_Int64Pad(t *testing.T) {
	r := &Rule{Pad: 1}
	err := r.ApplySettings(map[string]any{"pad": int64(3)})
	require.NoError(t, err)
	assert.Equal(t, 3, r.Pad)
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

func TestDetectPrefix_NoLeadingPipe(t *testing.T) {
	line := []byte("text without pipe")
	prefix := detectPrefix(line)
	assert.Equal(t, "", prefix)
}

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

// --- isSeparatorRow edge cases ---

func TestIsSeparatorRow_EmptyCells(t *testing.T) {
	assert.False(t, isSeparatorRow([]string{}))
}

func TestIsSeparatorRow_NonSeparatorCell(t *testing.T) {
	assert.False(t, isSeparatorRow([]string{"abc"}))
}

func TestIsSeparatorRow_MixedValid(t *testing.T) {
	assert.True(t, isSeparatorRow([]string{"---", ":---:", "---:"}))
}

// --- Fix with negative pad defaults ---

func TestFix_NegativePad_DefaultsTo1(t *testing.T) {
	src := "| a | b |\n|---|---|\n| 1 | 2 |\n"
	r := &Rule{Pad: -1}
	f := newTestFile(t, src)
	result := string(r.Fix(f))
	// Should use pad=1, producing padded output
	assert.Contains(t, result, "| a   | b   |")
}

// --- Check with negative pad ---

func TestCheck_NegativePad_DefaultsTo1(t *testing.T) {
	src := "| a | b |\n|---|---|\n| 1 | 2 |\n"
	r := &Rule{Pad: -1}
	f := newTestFile(t, src)
	diags := r.Check(f)
	// Negative pad defaults to 1; the table needs reformatting to match pad=1.
	assert.NotEmpty(t, diags, "negative pad should default to 1 and produce diagnostics")
}

// --- FormatString integration with blockquote ---

func TestFormatString_Blockquote(t *testing.T) {
	src := "> | a | bb |\n> |---|---|\n> | 1 | 2 |\n"
	result := FormatString(src, 1)
	for _, line := range strings.Split(strings.TrimRight(result, "\n"), "\n") {
		assert.True(t, strings.HasPrefix(line, "> "), "blockquote prefix preserved: %q", line)
	}
}
