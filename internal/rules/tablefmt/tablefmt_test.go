package tablefmt

import (
	"bytes"
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

func TestFindTables_FirstLineInCodeBlock_Skipped(t *testing.T) {
	// codeLines[1] = true means the very first line sits inside a code
	// block. findTables must skip it before attempting to parse a table
	// there.
	src := "| a | b |\n|---|---|\n| 1 | 2 |\n"
	lines := splitLines(src)
	tables := findTables(lines, map[int]bool{1: true})
	assert.Empty(t, tables, "no tables should be parsed when the header line is inside code")
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

// --- ParseSeparatorStyle coverage ---

func TestParseSeparatorStyle_Spaced(t *testing.T) {
	got, err := ParseSeparatorStyle("spaced", "any-rule")
	require.NoError(t, err)
	assert.Equal(t, SeparatorSpaced, got)
}

func TestParseSeparatorStyle_Compact(t *testing.T) {
	got, err := ParseSeparatorStyle("compact", "any-rule")
	require.NoError(t, err)
	assert.Equal(t, SeparatorCompact, got)
}

func TestParseSeparatorStyle_UnknownString_ErrorMentionsRuleAndValue(t *testing.T) {
	_, err := ParseSeparatorStyle("wide", "catalog")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "catalog:")
	assert.Contains(t, err.Error(), "\"wide\"")
}

func TestParseSeparatorStyle_NonString_ErrorMentionsTypeAndRule(t *testing.T) {
	_, err := ParseSeparatorStyle(42, "table-format")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "table-format:")
	assert.Contains(t, err.Error(), "must be a string")
	assert.Contains(t, err.Error(), "int")
}

// --- separatorAlignments coverage ---

func TestSeparatorAlignments_NoSeparatorReturnsNil(t *testing.T) {
	// formatTable's guard requires a parsed separator row, but the
	// helper is defensively coded to handle inputs without one. Pin
	// the nil result so the call site (computeColWidths) keeps its
	// len(aligns) check honest.
	rows := []row{
		{cells: []string{"a"}, isSeparator: false},
		{cells: []string{"b"}, isSeparator: false},
	}
	assert.Nil(t, separatorAlignments(rows))
}

func TestSeparatorAlignments_ReturnsFirstSeparatorRowAlignments(t *testing.T) {
	want := []align{alignLeft, alignCenter, alignRight}
	rows := []row{
		{cells: []string{"H", "H", "H"}, isSeparator: false},
		{cells: []string{":---", ":---:", "---:"}, isSeparator: true, alignments: want},
		{
			cells:       []string{":---", "---", "---"},
			isSeparator: true,
			alignments:  []align{alignLeft, alignNone, alignNone},
		},
	}
	assert.Equal(t, want, separatorAlignments(rows),
		"first separator row's alignments win")
}

// --- computeColWidths nil-aligns coverage ---

func TestComputeColWidths_NilAlignsFallsBackToAlignNone(t *testing.T) {
	// With aligns == nil every column should be treated as alignNone,
	// so the floor is 3 (the alignment-agnostic minimum) regardless of
	// cell content width.
	rows := []row{
		{cells: []string{"a", "bb"}, isSeparator: false},
	}
	widths := computeColWidths(rows, 2, nil, Config{Pad: 1})
	assert.Equal(t, []int{3, 3}, widths)
}

func TestComputeColWidths_AlignsShorterThanNumCols_PadsWithAlignNone(t *testing.T) {
	// A separator with fewer cells than the widest data row leaves the
	// trailing columns without an explicit alignment; they default to
	// alignNone (floor 3) instead of indexing past len(aligns).
	rows := []row{
		{cells: []string{"a", "b", "c"}, isSeparator: false},
	}
	widths := computeColWidths(rows, 3, []align{alignCenter}, Config{Pad: 1})
	assert.Equal(t, []int{5, 3, 3}, widths,
		"col 0 widened to 5 by center alignment; cols 1-2 fall back to alignNone")
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
	result := formatTable(tbl, Config{Pad: 1})
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
	src := "| a   | b      |\n| --- | ------ |\n| foo | barbaz |\n"
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
	// pad=0 collapses padding on both data and separator rows; spaced
	// and compact separators converge on the same all-dash form.
	src := "| a | b |\n|---|---|\n| 1 | 2 |\n"
	result := FormatString(src, 0)
	assert.Contains(t, result, "|a  |b  |")
	assert.Contains(t, result, "|---|---|")
}

func TestFormatString_Blockquote(t *testing.T) {
	src := "> | a | bb |\n> |---|---|\n> | 1 | 2 |\n"
	result := FormatString(src, 1)
	for _, line := range strings.Split(strings.TrimRight(result, "\n"), "\n") {
		assert.True(t, strings.HasPrefix(line, "> "), "blockquote prefix preserved: %q", line)
	}
}

// --- Regression: byte-identical tables are each rewritten ---

func TestFormatString_IdenticalTablesBothRewritten(t *testing.T) {
	// Two byte-identical, mis-formatted tables. A naïve bytes.Replace
	// pass would find both occurrences with the first call and replace
	// only one; the second would remain unformatted. Line-index
	// rewriting must visit each parsed table individually.
	src := "| a | b |\n" +
		"|---|---|\n" +
		"| foo | barbaz |\n" +
		"\n" +
		"| a | b |\n" +
		"|---|---|\n" +
		"| foo | barbaz |\n"
	out := FormatString(src, 1)
	canonical := strings.Count(out, "| a   | b      |")
	assert.Equal(t, 2, canonical, "both header rows must be reformatted; got:\n%s", out)
}

// --- Spaced (default) vs compact separator styles ---

func TestFormatString_DefaultSpacedSeparator(t *testing.T) {
	// The user's bug report: padded separator must be accepted by the
	// default Config, and Fix must produce the same form.
	src := "| Field | Details |\n" +
		"| ----- | ------- |\n" +
		"| abc   | abc     |\n"
	got := FormatString(src, 1)
	assert.Equal(t, src, got, "spaced-style table must be returned unchanged; got:\n%s", got)
}

func TestFormatStringWithConfig_CompactSeparator(t *testing.T) {
	// Opt-in compact style packs dashes flush against the pipes.
	src := "| Field | Details |\n" +
		"| ----- | ------- |\n" +
		"| abc   | abc     |\n"
	got := FormatStringWithConfig(src, Config{Pad: 1, SeparatorStyle: SeparatorCompact})
	assert.Contains(t, got, "|-------|---------|",
		"compact-style separator should drop the padding; got:\n%s", got)
}

func TestViolations_CompactStyle_FlagsSpacedTable(t *testing.T) {
	// A spaced separator should be flagged when the user opts into
	// compact style.
	src := "| Name   | Value |\n" +
		"| ------ | ----- |\n" +
		"| foo    | bar   |\n"
	got := Violations(splitLines(src), nil, Config{Pad: 1, SeparatorStyle: SeparatorCompact})
	require.Len(t, got, 1, "spaced separator must be a violation under compact style")
	assert.Contains(t, got[0].Message, "table is not formatted")
}

func TestViolations_SpacedStyle_FlagsCompactTable(t *testing.T) {
	src := "| Name   | Value |\n" +
		"|--------|-------|\n" +
		"| foo    | bar   |\n"
	got := Violations(splitLines(src), nil, Config{Pad: 1})
	require.Len(t, got, 1, "compact separator must be a violation under spaced (default) style")
	assert.Contains(t, got[0].Message, "table is not formatted")
}

func TestFormatStringWithConfig_SpacedAlignment(t *testing.T) {
	// Header "Header" widens the column to 6 so each alignment arm has
	// room for the colon plus three dashes.
	cases := map[string]struct {
		sep  string
		want string
	}{
		"left":   {sep: "|:---|", want: "| :----- |"},
		"right":  {sep: "|---:|", want: "| -----: |"},
		"center": {sep: "|:---:|", want: "| :----: |"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			src := "| Header |\n" + tc.sep + "\n| a |\n"
			out := FormatStringWithConfig(src, Config{Pad: 1})
			assert.Contains(t, out, tc.want,
				"spaced %s-aligned separator; got:\n%s", name, out)
		})
	}
}

// TestFormatStringWithConfig_SpacedAlignment_NarrowColumn_Min3Dashes
// pins the cross-flavor minimum: every separator cell must contain at
// least three hyphens regardless of column width, so the output parses
// as a table under strict GFM flavors (markdown-it, pandoc) that reject
// `:--`, `:-:`, and `--:`.
func TestFormatStringWithConfig_SpacedAlignment_NarrowColumn_Min3Dashes(t *testing.T) {
	cases := map[string]struct {
		sep     string
		wantSep string
		wantRow string
	}{
		"none":   {sep: "|---|", wantSep: "| --- |", wantRow: "| a   |"},
		"left":   {sep: "|:---|", wantSep: "| :--- |", wantRow: "| a    |"},
		"right":  {sep: "|---:|", wantSep: "| ---: |", wantRow: "| a    |"},
		"center": {sep: "|:---:|", wantSep: "| :---: |", wantRow: "| a     |"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			src := "| h |\n" + tc.sep + "\n| a |\n"
			out := FormatStringWithConfig(src, Config{Pad: 1})
			assert.Contains(t, out, tc.wantSep,
				"spaced %s separator needs 3 dashes; got:\n%s", name, out)
			assert.Contains(t, out, tc.wantRow,
				"data row must align to widened column; got:\n%s", name+":\n"+out)
		})
	}
}

// TestFormatStringWithConfig_CompactAlignment_NarrowColumn_Min3Dashes
// is the compact-mode counterpart: with pad=0 the cell content equals
// colWidth, so the same minimum-dash rule must widen the column.
func TestFormatStringWithConfig_CompactAlignment_NarrowColumn_Min3Dashes(t *testing.T) {
	cases := map[string]struct {
		sep     string
		wantSep string
	}{
		"none":   {sep: "|---|", wantSep: "|---|"},
		"left":   {sep: "|:---|", wantSep: "|:---|"},
		"right":  {sep: "|---:|", wantSep: "|---:|"},
		"center": {sep: "|:---:|", wantSep: "|:---:|"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			src := "|h|\n" + tc.sep + "\n|a|\n"
			out := FormatStringWithConfig(src, Config{Pad: 0, SeparatorStyle: SeparatorCompact})
			assert.Contains(t, out, tc.wantSep,
				"compact %s separator needs 3 dashes; got:\n%s", name, out)
		})
	}
}

// --- Regression: table-shaped text inside a skipped code block is left alone ---

func TestFormatLines_TableInsideSkippedCodeBlock_NotRewritten(t *testing.T) {
	// A table-shaped block sits inside a fenced code block (lines 2-4
	// are marked as code via codeLines). An identical real table
	// follows the fence (lines 7-9). The fenced text must stay byte-
	// for-byte; the real table downstream must be reformatted.
	src := []byte("" +
		"```\n" + // 1
		"| a | b |\n" + // 2 (code)
		"|---|---|\n" + // 3 (code)
		"| foo | barbaz |\n" + // 4 (code)
		"```\n" + // 5
		"\n" + // 6
		"| a | b |\n" + // 7
		"|---|---|\n" + // 8
		"| foo | barbaz |\n") // 9
	lines := splitLines(string(src))
	codeLines := map[int]bool{2: true, 3: true, 4: true}

	out := FormatLines(src, lines, codeLines, Config{Pad: 1})
	outStr := string(out)

	// Inside the fence: the original mis-formatted rows must survive.
	assert.Contains(t, outStr, "```\n| a | b |\n|---|---|\n| foo | barbaz |\n```",
		"fenced contents must not be reformatted; got:\n%s", outStr)
	// Downstream: the real table must be reformatted exactly once.
	assert.Equal(t, 1, strings.Count(outStr, "| a   | b      |"),
		"real table should be reformatted to canonical width; got:\n%s", outStr)
}

// --- Violations ---

func TestViolations_FormattedTable_None(t *testing.T) {
	src := "| Name   | Description               |\n" +
		"| ------ | ------------------------- |\n" +
		"| foo    | A short one               |\n" +
		"| barbaz | A longer description here |\n"
	lines := splitLines(src)
	got := Violations(lines, nil, Config{Pad: 1})
	assert.Empty(t, got)
}

func TestViolations_MisalignedTable_One(t *testing.T) {
	src := "| Name | Description |\n" +
		"|---|---|\n" +
		"| foo | A short one |\n" +
		"| barbaz | A longer description here |\n"
	lines := splitLines(src)
	got := Violations(lines, nil, Config{Pad: 1})
	require.Len(t, got, 1)
	assert.Equal(t, 1, got[0].StartLine)
	assert.Contains(t, got[0].Message, "table is not formatted")
}

func TestViolations_NegativePadDefaultsTo1(t *testing.T) {
	src := "| a | b |\n|---|---|\n| 1 | 2 |\n"
	lines := splitLines(src)
	got := Violations(lines, nil, Config{Pad: -1})
	assert.NotEmpty(t, got, "negative pad should default to 1 and produce violations")
}

// --- FormatLines ---

func TestFormatLines_NoTables_ReturnsCopyOfSource(t *testing.T) {
	src := []byte("# Heading\n\nSome text.\n")
	lines := splitLines(string(src))
	result := FormatLines(src, lines, nil, Config{Pad: 1})
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
	result := FormatLines(src, lines, nil, Config{Pad: 1})
	assert.Contains(t, string(result), "| a   | b      |")
}

func TestFormatLines_AlreadyFormattedTableAmongOthers(t *testing.T) {
	// Two tables: the first is already canonical, the second is not.
	// The reverse-iteration loop visits the second first (rewriting it)
	// and then the first (hitting the tableEqual `continue` branch).
	src := []byte("" +
		"| a   | b   |\n" +
		"| --- | --- |\n" +
		"| foo | bar |\n" +
		"\n" +
		"| x | y |\n" +
		"|---|---|\n" +
		"| 1 | 2 |\n")
	lines := splitLines(string(src))
	result := FormatLines(src, lines, nil, Config{Pad: 1})
	// First (canonical) table is preserved byte-for-byte.
	assert.Contains(t, string(result), "| a   | b   |")
	assert.Contains(t, string(result), "| foo | bar |")
	// Second table is rewritten.
	assert.Contains(t, string(result), "| x   | y   |")
}

func TestFormatLines_NegativePadDefaultsTo1(t *testing.T) {
	src := []byte("| a | b |\n|---|---|\n| 1 | 2 |\n")
	lines := splitLines(string(src))
	result := FormatLines(src, lines, nil, Config{Pad: -1})
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

// --- writeSeparatorRow: each alignment indicator ---

func TestWriteSeparatorRow_AlignmentIndicators(t *testing.T) {
	// Each alignment marker exercises a different switch arm in
	// writeSeparatorRow: alignLeft (`:---`), alignCenter (`:---:`), and
	// alignRight (`---:`). FormatString reflows the table so the
	// resulting separator carries the markers at the canonical widths.
	src := "| Left | Center | Right |\n" +
		"|:---|:---:|---:|\n" +
		"| a | b | c |\n"
	out := FormatString(src, 1)
	assert.Contains(t, out, "| :--- | :----: | ----: |",
		"expected separator preserves all three alignment markers; got:\n%s", out)
}

func TestWriteSeparatorRow_CompactAlignmentIndicators(t *testing.T) {
	// SeparatorCompact places alignment colons flush against the pipes.
	src := "| Left | Center | Right |\n" +
		"|:---|:---:|---:|\n" +
		"| a | b | c |\n"
	out := FormatStringWithConfig(src, Config{Pad: 1, SeparatorStyle: SeparatorCompact})
	assert.Contains(t, out, "|:-----|:------:|------:|",
		"expected compact separator preserves all three alignment markers; got:\n%s", out)
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

// --- normalizeConfig ---

// TestNormalizeConfig pins every reachable branch directly. The
// FormatString and Violations integration paths feed this helper
// implicitly, but neither asserts on the returned Config shape,
// so a regression in the negative-Pad fallback would only show
// in downstream behaviour. The direct unit pin makes it cheap
// to bisect.
func TestNormalizeConfig(t *testing.T) {
	t.Run("negative Pad falls back to 1", func(t *testing.T) {
		got := normalizeConfig(Config{Pad: -5, SeparatorStyle: SeparatorCompact})
		if got.Pad != 1 {
			t.Errorf("Pad = %d, want 1", got.Pad)
		}
		if got.SeparatorStyle != SeparatorCompact {
			t.Errorf("SeparatorStyle = %v, want SeparatorCompact "+
				"(must not be reset alongside Pad)", got.SeparatorStyle)
		}
	})
	t.Run("zero Pad passes through unchanged", func(t *testing.T) {
		got := normalizeConfig(Config{Pad: 0, SeparatorStyle: SeparatorSpaced})
		if got.Pad != 0 {
			t.Errorf("Pad = %d, want 0 (zero is a valid, kept-as-is value)", got.Pad)
		}
	})
	t.Run("positive Pad passes through unchanged", func(t *testing.T) {
		got := normalizeConfig(Config{Pad: 3, SeparatorStyle: SeparatorSpaced})
		if got.Pad != 3 {
			t.Errorf("Pad = %d, want 3", got.Pad)
		}
		if got.SeparatorStyle != SeparatorSpaced {
			t.Errorf("SeparatorStyle = %v, want SeparatorSpaced", got.SeparatorStyle)
		}
	})
}

// --- rebuildWithFormattedTables ---

// TestRebuildWithFormattedTables_RewritesNonConformingTable pins
// the splice branch directly: a non-canonical table at the
// specified startLine has every row replaced by the formatted
// table's rawLines. The integration test
// (TestFormatLines_AlreadyFormattedTableAmongOthers) covers the
// same branch through FormatLines, but driving the helper
// directly lets us assert the byte-level output on a
// hand-constructed table literal — so a regression that breaks
// the indexing (e.g. off-by-one on startLine) lands here first.
func TestRebuildWithFormattedTables_RewritesNonConformingTable(t *testing.T) {
	// Non-conforming table at lines 1..3 (1-based). The header has
	// no padding around cell content, so formatTable will rewrite
	// all three rows. parseAlignments needs the separator cell
	// alignments to be set so the formatted output reproduces the
	// expected canonical layout.
	lines := [][]byte{
		[]byte("|a|b|"),
		[]byte("|---|---|"),
		[]byte("|1|2|"),
	}
	tbl := table{
		startLine: 1,
		rawLines:  [][]byte{lines[0], lines[1], lines[2]},
		prefix:    "",
		rows: []row{
			{cells: []string{"a", "b"}},
			{
				cells:       []string{"---", "---"},
				isSeparator: true,
				alignments:  []align{alignNone, alignNone},
			},
			{cells: []string{"1", "2"}},
		},
	}
	got := rebuildWithFormattedTables(lines, []table{tbl}, Config{Pad: 1})
	// Pad=1 with min-3-dash columns produces three-space content
	// areas (1 pad + 1 content char + 2 pad-out + 1 pad).
	want := "| a   | b   |\n| --- | --- |\n| 1   | 2   |"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", string(got), want)
	}
}

// TestRebuildWithFormattedTables_SkipsAlreadyFormatted pins the
// `tableEqual → continue` branch directly: a table whose
// rawLines already match the canonical layout under the
// chosen config must be passed through unchanged. We seed the
// input with the EXACT bytes formatTable would produce (Pad=1,
// 3-dash separator) so the equal-check fires.
func TestRebuildWithFormattedTables_SkipsAlreadyFormatted(t *testing.T) {
	// Canonical form under Config{Pad:1}: column floor is 3, so
	// "a" becomes "a   " in the content area. Single-char data
	// also widens to the 3-char floor.
	canonical := [][]byte{
		[]byte("| a   | b   |"),
		[]byte("| --- | --- |"),
		[]byte("| 1   | 2   |"),
	}
	tbl := table{
		startLine: 1,
		rawLines:  canonical,
		prefix:    "",
		rows: []row{
			{cells: []string{"a", "b"}},
			{
				cells:       []string{"---", "---"},
				isSeparator: true,
				alignments:  []align{alignNone, alignNone},
			},
			{cells: []string{"1", "2"}},
		},
	}
	got := rebuildWithFormattedTables(canonical, []table{tbl}, Config{Pad: 1})
	want := "| a   | b   |\n| --- | --- |\n| 1   | 2   |"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", string(got), want)
	}
}

// TestRebuildWithFormattedTables_NoTablesReturnsJoinedSource pins
// the no-table fast path: when tables is empty, the helper
// returns the input lines joined with newlines, with no other
// transformation. FormatLines short-circuits before calling this
// helper, so the empty-tables branch was only reachable
// directly.
func TestRebuildWithFormattedTables_NoTablesReturnsJoinedSource(t *testing.T) {
	lines := [][]byte{[]byte("plain"), []byte("text"), []byte("")}
	got := rebuildWithFormattedTables(lines, nil, Config{Pad: 1})
	want := "plain\ntext\n"
	if string(got) != want {
		t.Errorf("got %q, want %q", string(got), want)
	}
}

// --- table struct literal construction ---

// TestTable_StructFieldsRoundTrip pins that every documented
// field on the table struct round-trips through formatTable's
// reader code. Constructing the struct directly hits the
// definition site, and the field values are observed by
// asserting on tableEqual + the formatted output. Without this
// the struct's field initialisation lines were only ever set by
// tryParseTable (a different file), so the package's parse
// shape and the format shape were tested independently.
func TestTable_StructFieldsRoundTrip(t *testing.T) {
	tbl := table{
		startLine: 5,
		rawLines:  [][]byte{[]byte("| a |"), []byte("| - |"), []byte("| 1 |")},
		prefix:    "> ",
		rows: []row{
			{cells: []string{"a"}},
			{cells: []string{"-"}, isSeparator: true, alignments: []align{alignNone}},
			{cells: []string{"1"}},
		},
	}
	if tbl.startLine != 5 {
		t.Errorf("startLine = %d, want 5", tbl.startLine)
	}
	if tbl.prefix != "> " {
		t.Errorf("prefix = %q, want %q", tbl.prefix, "> ")
	}
	if len(tbl.rawLines) != 3 {
		t.Errorf("len(rawLines) = %d, want 3", len(tbl.rawLines))
	}
	if len(tbl.rows) != 3 {
		t.Errorf("len(rows) = %d, want 3", len(tbl.rows))
	}
	// formatTable preserves the prefix in every output row.
	got := formatTable(tbl, Config{Pad: 1})
	for _, line := range got.rawLines {
		if !bytes.HasPrefix(line, []byte("> ")) {
			t.Errorf("formatted row %q must keep prefix", string(line))
		}
	}
	// tableEqual must treat identical tables as equal.
	if !tableEqual(tbl, tbl) {
		t.Errorf("tableEqual(tbl, tbl) must be true")
	}
}
