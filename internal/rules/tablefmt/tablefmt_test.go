package tablefmt

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Cell parsing tests ---

func TestSplitRow_Basic(t *testing.T) {
	cells := splitRow("| a | b | c |")
	want := []string{"a", "b", "c"}
	assertCells(t, want, cells)
}

func TestSplitRow_EscapedPipe(t *testing.T) {
	cells := splitRow(`| a \| b | c |`)
	want := []string{`a \| b`, "c"}
	assertCells(t, want, cells)
}

func TestSplitRow_EmptyCells(t *testing.T) {
	cells := splitRow("| | b | |")
	want := []string{"", "b", ""}
	assertCells(t, want, cells)
}

func TestSplitRow_InlineCode(t *testing.T) {
	cells := splitRow("| `code` | normal |")
	want := []string{"`code`", "normal"}
	assertCells(t, want, cells)
}

func TestSplitRow_Link(t *testing.T) {
	cells := splitRow("| [text](url) | normal |")
	want := []string{"[text](url)", "normal"}
	assertCells(t, want, cells)
}

func TestSplitRow_SingleColumn(t *testing.T) {
	cells := splitRow("| only |")
	want := []string{"only"}
	assertCells(t, want, cells)
}

// --- Display width tests ---

func TestDisplayWidth_ASCII(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"hello", 5},
		{"", 0},
		{"abc", 3},
	}
	for _, tt := range tests {
		got := displayWidth(tt.input)
		assert.Equal(t, tt.want, got, "displayWidth(%q) = %d, want %d", tt.input, got, tt.want)
	}
}

func TestDisplayWidth_Multibyte(t *testing.T) {
	got := displayWidth("café")
	assert.Equal(t, 4, got, "displayWidth(café) = %d, want 4", got)
}

func TestDisplayWidth_Emoji(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"✅", 2},
		{"🔲", 2},
		{"🔳", 2},
		{"✅ done", 7},
	}
	for _, tt := range tests {
		got := displayWidth(tt.input)
		assert.Equal(t, tt.want, got, "displayWidth(%q) = %d, want %d", tt.input, got, tt.want)
	}
}

func TestDisplayWidth_Link(t *testing.T) {
	got := displayWidth("[text](https://example.com)")
	assert.Equal(t, 27, got, "displayWidth counts raw characters including URL")
}

func TestDisplayWidth_InlineCode(t *testing.T) {
	got := displayWidth("`code`")
	assert.Equal(t, 6, got, "displayWidth counts backticks")
}

func TestDisplayWidth_Bold(t *testing.T) {
	got := displayWidth("**bold**")
	assert.Equal(t, 8, got, "displayWidth counts asterisks")
}

func TestDisplayWidth_Italic(t *testing.T) {
	got := displayWidth("*italic*")
	assert.Equal(t, 8, got, "displayWidth counts asterisks")
}

func TestDisplayWidth_Image(t *testing.T) {
	got := displayWidth("![alt text](image.png)")
	assert.Equal(t, 22, got, "displayWidth counts raw characters including URL")
}

func TestDisplayWidth_Strikethrough(t *testing.T) {
	got := displayWidth("~~deleted~~")
	assert.Equal(t, 11, got, "displayWidth counts tildes")
}

func TestDisplayWidth_Mixed(t *testing.T) {
	got := displayWidth("see [text](url) for details")
	assert.Equal(t, 27, got, "displayWidth counts raw characters")
}

func TestDisplayWidth_CJK(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"日本語", 6},  // 3 CJK chars × 2 columns each
		{"a日b", 4},  // 1 + 2 + 1
		{"中文测试", 8}, // 4 CJK chars × 2
	}
	for _, tt := range tests {
		got := displayWidth(tt.input)
		assert.Equal(t, tt.want, got, "displayWidth(%q) = %d, want %d", tt.input, got, tt.want)
	}
}

// --- Table detection tests ---

func TestFindTables_Basic(t *testing.T) {
	src := "| a | b |\n|---|---|\n| 1 | 2 |\n"
	lines := splitLines(src)
	tables := findTables(lines, nil)
	require.Len(t, tables, 1, "expected 1 table, got %d", len(tables))
	if tables[0].startLine != 1 {
		t.Errorf("start line = %d, want 1", tables[0].startLine)
	}
	assert.Len(t, tables[0].rows, 3, "rows = %d, want 3", len(tables[0].rows))
}

func TestFindTables_NoTable(t *testing.T) {
	src := "# Heading\n\nSome text.\n"
	lines := splitLines(src)
	tables := findTables(lines, nil)
	assert.Len(t, tables, 0, "expected 0 tables, got %d", len(tables))
}

func TestFindTables_TwoTables(t *testing.T) {
	src := "| a | b |\n|---|---|\n| 1 | 2 |\n\n| x | y |\n|---|---|\n| 3 | 4 |\n"
	lines := splitLines(src)
	tables := findTables(lines, nil)
	require.Len(t, tables, 2, "expected 2 tables, got %d", len(tables))
}

func TestFindTables_SingleColumn(t *testing.T) {
	src := "| a |\n|---|\n| 1 |\n"
	lines := splitLines(src)
	tables := findTables(lines, nil)
	require.Len(t, tables, 1, "expected 1 table, got %d", len(tables))
	assert.Len(t, tables[0].rows[0].cells, 1, "columns = %d, want 1", len(tables[0].rows[0].cells))
}

// --- Separator parsing tests ---

func TestIsSeparatorRow(t *testing.T) {
	tests := []struct {
		cells []string
		want  bool
	}{
		{[]string{"---", "---"}, true},
		{[]string{":---", "---:"}, true},
		{[]string{":---:"}, true},
		{[]string{"abc", "---"}, false},
		{[]string{""}, false},
	}
	for _, tt := range tests {
		got := isSeparatorRow(tt.cells)
		assert.Equal(t, tt.want, got, "isSeparatorRow(%v) = %v, want %v", tt.cells, got, tt.want)
	}
}

func TestIsSeparatorRow_EmptyCells(t *testing.T) {
	assert.False(t, isSeparatorRow([]string{}))
}

func TestIsSeparatorRow_NonSeparatorCell(t *testing.T) {
	assert.False(t, isSeparatorRow([]string{"abc"}))
}

func TestIsSeparatorRow_MixedValid(t *testing.T) {
	assert.True(t, isSeparatorRow([]string{"---", ":---:", "---:"}))
}

func TestParseAlignments(t *testing.T) {
	cells := []string{"---", ":---", "---:", ":---:"}
	aligns := parseAlignments(cells)
	want := []align{alignNone, alignLeft, alignRight, alignCenter}
	require.Len(t, aligns, len(want), "len = %d, want %d", len(aligns), len(want))
	for i := range want {
		if aligns[i] != want[i] {
			t.Errorf("align[%d] = %d, want %d", i, aligns[i], want[i])
		}
	}
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

func TestDetectPrefix_BlockquoteNakedGtBeforeGt(t *testing.T) {
	// ">>| a |": first iteration sees ">>", trimmed[1]=='>', consumes one ">".
	// Second iteration sees ">| a | b |", trimmed[1]=='|', neither branch
	// matches → break. Result: ">".
	line := []byte(">>| a | b |")
	prefix := detectPrefix(line)
	assert.Equal(t, ">", prefix)
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

// --- tryParseTable edge cases ---

func TestTryParseTable_StartAtEnd(t *testing.T) {
	// An empty lines slice means tryParseTable is never called, but we can
	// call it directly with start == len(lines).
	lines := [][]byte{[]byte("| a | b |")}
	tbl, end := tryParseTable(lines, 1, nil) // start == len(lines)
	assert.Nil(t, tbl)
	assert.Equal(t, 1, end)
}

func TestTryParseTable_OnlyHeaderNoSeparator(t *testing.T) {
	lines := [][]byte{[]byte("| a | b |")} // only 1 line
	tbl, _ := tryParseTable(lines, 0, nil)
	assert.Nil(t, tbl, "expected nil when there's no separator line")
}

func TestTryParseTable_SecondLineNotTableRow(t *testing.T) {
	lines := [][]byte{
		[]byte("| a | b |"),
		[]byte("some plain text"),
	}
	tbl, _ := tryParseTable(lines, 0, nil)
	assert.Nil(t, tbl, "expected nil when second line is not a table row")
}

func TestTryParseTable_SecondLineNotSeparator(t *testing.T) {
	lines := [][]byte{
		[]byte("| a | b |"),
		[]byte("| c | d |"),
	}
	tbl, _ := tryParseTable(lines, 0, nil)
	assert.Nil(t, tbl, "expected nil when second line is not a separator")
}

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

// --- formatTable: len(tbl.rows) < 2 ---

func TestFormatTable_LessThanTwoRows(t *testing.T) {
	tbl := table{
		startLine: 1,
		rawLines:  [][]byte{[]byte("| a | b |")},
		rows:      []row{{cells: []string{"a", "b"}}},
	}
	result := formatTable(tbl, 1)
	assert.Equal(t, tbl.rawLines, result.rawLines)
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
	assert.Equal(t, "table is not formatted", msg)
}

// --- tableEqual: different lengths ---

func TestTableEqual_DifferentLengths(t *testing.T) {
	a := table{rawLines: [][]byte{[]byte("| a |"), []byte("|---|")}}
	b := table{rawLines: [][]byte{[]byte("| a |")}}
	assert.False(t, tableEqual(a, b))
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
	assert.Contains(t, result, "| a   | b   |")
	assert.Contains(t, result, "| x   | y   |")
}

func TestFormatString_Pad0(t *testing.T) {
	src := "| a | b |\n|---|---|\n| 1 | 2 |\n"
	result := FormatString(src, 0)
	assert.Contains(t, result, "|a  |b  |")
}

func TestFormatString_Blockquote(t *testing.T) {
	src := "> | a | bb |\n> |---|---|\n> | 1 | 2 |\n"
	result := FormatString(src, 1)
	for _, line := range strings.Split(strings.TrimRight(result, "\n"), "\n") {
		assert.True(t, strings.HasPrefix(line, "> "), "blockquote prefix preserved: %q", line)
	}
}

// --- Violations ---

func TestViolations_FormattedTable_None(t *testing.T) {
	src := "| Name   | Description               |\n" +
		"|--------|---------------------------|\n" +
		"| foo    | A short one               |\n" +
		"| barbaz | A longer description here |\n"
	lines := splitLines(src)
	got := Violations(lines, nil, 1)
	assert.Empty(t, got)
}

func TestViolations_MisalignedTable_One(t *testing.T) {
	src := "| Name | Description |\n" +
		"|---|---|\n" +
		"| foo | A short one |\n" +
		"| barbaz | A longer description here |\n"
	lines := splitLines(src)
	got := Violations(lines, nil, 1)
	require.Len(t, got, 1)
	assert.Equal(t, 1, got[0].StartLine)
	assert.Contains(t, got[0].Message, "table is not formatted")
}

func TestViolations_NegativePadDefaultsTo1(t *testing.T) {
	src := "| a | b |\n|---|---|\n| 1 | 2 |\n"
	lines := splitLines(src)
	got := Violations(lines, nil, -1)
	assert.NotEmpty(t, got, "negative pad should default to 1 and produce violations")
}

// --- FormatLines ---

func TestFormatLines_NoTables_ReturnsCopyOfSource(t *testing.T) {
	src := []byte("# Heading\n\nSome text.\n")
	lines := splitLines(string(src))
	result := FormatLines(src, lines, nil, 1)
	assert.Equal(t, src, result)
	// Result must be a copy, not the same slice header.
	if len(src) > 0 {
		result[0] = 'X'
		assert.Equal(t, byte('#'), src[0], "mutating result must not mutate source")
	}
}

func TestFormatLines_RewritesTables(t *testing.T) {
	src := []byte("| a | b |\n|---|---|\n| foo | barbaz |\n")
	lines := splitLines(string(src))
	result := FormatLines(src, lines, nil, 1)
	assert.Contains(t, string(result), "| a   | b      |")
}

func TestFormatLines_NegativePadDefaultsTo1(t *testing.T) {
	src := []byte("| a | b |\n|---|---|\n| 1 | 2 |\n")
	lines := splitLines(string(src))
	result := FormatLines(src, lines, nil, -1)
	assert.Contains(t, string(result), "| a   | b   |")
}

// --- writeSeparatorRow: aligns extended to numCols ---

func TestWriteSeparatorRow_ExtendsAlignments(t *testing.T) {
	// Header has 3 columns but separator has 2; formatTable should still
	// produce a 3-column separator. Exercised via FormatString.
	src := "| a | b | c |\n|---|---|\n| 1 | 2 | 3 |\n"
	out := FormatString(src, 1)
	assert.Contains(t, out, "| a   | b   | c   |")
}

// --- Helper functions ---

func splitLines(s string) [][]byte {
	parts := strings.Split(s, "\n")
	// Remove trailing empty element from trailing newline.
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	lines := make([][]byte, len(parts))
	for i, p := range parts {
		lines[i] = []byte(p)
	}
	return lines
}

func assertCells(t *testing.T, want, got []string) {
	t.Helper()
	require.Len(t, got, len(want), "cells: got %d, want %d\n  got:  %v\n  want: %v", len(got), len(want), got, want)
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("cell[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
