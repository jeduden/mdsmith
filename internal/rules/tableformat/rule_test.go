package tableformat

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"

	"github.com/jeduden/mdsmith/internal/lint"

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

func TestFix_LinksWithVaryingURLLengths(t *testing.T) {
	// Regression test for #65: links with different URL lengths must
	// produce consistent column widths so trailing | aligns.
	src := "| Name | Link |\n" +
		"|---|---|\n" +
		"| a | [short](x.md) |\n" +
		"| b | [long link text](https://example.com/very/long/path.md) |\n"
	r := &Rule{Pad: 1}
	f := newTestFile(t, src)
	fixed := r.Fix(f)
	lines := strings.Split(string(fixed), "\n")
	// All table rows must have the same display width (terminal columns).
	widths := map[int]bool{}
	for i := 0; i < 4; i++ {
		widths[runewidth.StringWidth(lines[i])] = true
	}
	assert.Len(t, widths, 1, "all rows should have same display width, got lines:\n%s", strings.Join(lines[:4], "\n"))
}

func TestFix_MixedEmojiAndLinks(t *testing.T) {
	src := "| Status | Task |\n" +
		"|---|---|\n" +
		"| ✅ | [Deploy](deploy.md) |\n" +
		"| 🔲 | [Build pipeline](build-pipeline.md) |\n"
	r := &Rule{Pad: 1}
	f := newTestFile(t, src)
	fixed := r.Fix(f)
	// All table rows must have the same display width (terminal columns),
	// even though byte lengths differ due to emoji encoding.
	lines := strings.Split(string(fixed), "\n")
	widths := map[int]bool{}
	for i := 0; i < 4; i++ {
		widths[runewidth.StringWidth(lines[i])] = true
	}
	assert.Len(t, widths, 1, "all rows should have same display width, got lines:\n%s", strings.Join(lines[:4], "\n"))
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

func TestCheck_TableInsideCodeBlock_NoDiagnostic(t *testing.T) {
	src := "# Example\n\n" +
		"```markdown\n" +
		"| a | b |\n" +
		"|---|---|\n" +
		"| 1 | 2 |\n" +
		"```\n"
	r := &Rule{Pad: 1}
	f := newTestFile(t, src)
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Errorf("expected 0 diagnostics for table inside code block, got %d", len(diags))
		for _, d := range diags {
			t.Logf("  line %d: %s", d.Line, d.Message)
		}
	}
}

// --- Check tests ---

func TestCheck_FormattedTable_NoDiagnostics(t *testing.T) {
	src := "| Name   | Description               |\n" +
		"|--------|---------------------------|\n" +
		"| foo    | A short one               |\n" +
		"| barbaz | A longer description here |\n"
	r := &Rule{Pad: 1}
	f := newTestFile(t, src)
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Errorf("expected 0 diagnostics, got %d", len(diags))
		for _, d := range diags {
			t.Logf("  line %d: %s", d.Line, d.Message)
		}
	}
}

func TestCheck_MisalignedTable_OneDiagnostic(t *testing.T) {
	src := "| Name | Description |\n" +
		"|---|---|\n" +
		"| foo | A short one |\n" +
		"| barbaz | A longer description here |\n"
	r := &Rule{Pad: 1}
	f := newTestFile(t, src)
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	if diags[0].Line != 1 {
		t.Errorf("diagnostic line = %d, want 1", diags[0].Line)
	}
	if diags[0].Message != "table is not formatted" {
		t.Errorf("message = %q, want %q", diags[0].Message, "table is not formatted")
	}
}

func TestCheck_ShortSeparator_Flagged(t *testing.T) {
	src := "| Name   | Desc   |\n" +
		"|---|---|\n" +
		"| foo    | bar    |\n"
	r := &Rule{Pad: 1}
	f := newTestFile(t, src)
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
}

// --- Fix tests ---

func TestFix_BasicAlignment(t *testing.T) {
	src := "| Name | Description |\n" +
		"|---|---|\n" +
		"| foo | A short one |\n" +
		"| barbaz | A longer description here |\n"
	want := "| Name   | Description               |\n" +
		"|--------|---------------------------|\n" +
		"| foo    | A short one               |\n" +
		"| barbaz | A longer description here |\n"
	r := &Rule{Pad: 1}
	f := newTestFile(t, src)
	got := string(r.Fix(f))
	assert.Equal(t, want, got, "Fix:\ngot:\n%s\nwant:\n%s", got, want)
}

func TestFix_AlignmentIndicators(t *testing.T) {
	src := "| Left | Center | Right |\n" +
		"|:---|:---:|---:|\n" +
		"| a | b | c |\n"
	r := &Rule{Pad: 1}
	f := newTestFile(t, src)
	got := string(r.Fix(f))

	// Verify alignment indicators are preserved.
	lines := strings.Split(got, "\n")
	sep := lines[1]
	assert.Contains(t, sep, ":", "alignment indicators not preserved in separator: %q", sep)
	// Left alignment: |:---
	assert.Contains(t, sep, "|:", "left alignment not preserved: %q", sep)
	// Right alignment: ---:|
	assert.Contains(t, sep, ":|", "right alignment not preserved: %q", sep)
}

func TestFix_PreservesContentOutsideTable(t *testing.T) {
	src := "# Title\n\n" +
		"Some text before.\n\n" +
		"| a | b |\n" +
		"|---|---|\n" +
		"| 1 | 2 |\n\n" +
		"Text after.\n"
	r := &Rule{Pad: 1}
	f := newTestFile(t, src)
	got := string(r.Fix(f))

	if !strings.HasPrefix(got, "# Title\n\nSome text before.\n\n") {
		t.Error("content before table not preserved")
	}
	if !strings.HasSuffix(got, "\n\nText after.\n") {
		t.Error("content after table not preserved")
	}
}

func TestFix_EmptyCells(t *testing.T) {
	src := "| a | b |\n" +
		"|---|---|\n" +
		"| | x |\n"
	r := &Rule{Pad: 1}
	f := newTestFile(t, src)
	got := string(r.Fix(f))
	if !strings.Contains(got, "|     |") || !strings.Contains(got, "| x   |") {
		// With pad=1 and minWidth=3: empty cell = "| " + "   " + " |"
		t.Errorf("empty cell not handled correctly:\n%s", got)
	}
}

func TestFix_EscapedPipes(t *testing.T) {
	src := "| Content | Note |\n" +
		"|---|---|\n" +
		`| a \| b | c |` + "\n"
	r := &Rule{Pad: 1}
	f := newTestFile(t, src)
	got := string(r.Fix(f))
	assert.Contains(t, got, `a \| b`, "escaped pipe not preserved:\n%s", got)
}

func TestFix_SingleColumn(t *testing.T) {
	src := "| a |\n|---|\n| b |\n"
	r := &Rule{Pad: 1}
	f := newTestFile(t, src)
	got := string(r.Fix(f))
	// Single column should be properly formatted.
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	require.Len(t, lines, 3, "expected 3 lines, got %d", len(lines))
}

// --- Contextual table tests ---

func TestFix_BlockquoteTable(t *testing.T) {
	src := "> | a | b |\n" +
		"> |---|---|\n" +
		"> | 1 | 22 |\n"
	r := &Rule{Pad: 1}
	f := newTestFile(t, src)
	got := string(r.Fix(f))
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, "> ") {
			t.Errorf("blockquote prefix not preserved: %q", line)
		}
	}
}

func TestFix_IndentedTable(t *testing.T) {
	src := "  | a | b |\n" +
		"  |---|---|\n" +
		"  | 1 | 22 |\n"
	r := &Rule{Pad: 1}
	f := newTestFile(t, src)
	got := string(r.Fix(f))
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, "  ") {
			t.Errorf("indentation not preserved: %q", line)
		}
	}
}

func TestFix_NestedBlockquote(t *testing.T) {
	src := "> > | a | bb |\n" +
		"> > |---|---|\n" +
		"> > | 1 | 2 |\n"
	r := &Rule{Pad: 1}
	f := newTestFile(t, src)
	got := string(r.Fix(f))
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, "> > ") {
			t.Errorf("nested blockquote prefix not preserved: %q", line)
		}
	}
}

// --- Settings tests ---

func TestApplySettings_ValidPad(t *testing.T) {
	r := &Rule{Pad: 1}
	err := r.ApplySettings(map[string]any{"pad": 2})
	require.NoError(t, err, "unexpected error: %v", err)
	if r.Pad != 2 {
		t.Errorf("pad = %d, want 2", r.Pad)
	}
}

func TestApplySettings_InvalidPad(t *testing.T) {
	r := &Rule{Pad: 1}
	err := r.ApplySettings(map[string]any{"pad": "two"})
	require.Error(t, err, "expected error for non-integer pad")
}

func TestApplySettings_NegativePad(t *testing.T) {
	r := &Rule{Pad: 1}
	err := r.ApplySettings(map[string]any{"pad": -1})
	require.Error(t, err, "expected error for negative pad")
}

func TestApplySettings_UnknownSetting(t *testing.T) {
	r := &Rule{Pad: 1}
	err := r.ApplySettings(map[string]any{"unknown": true})
	require.Error(t, err, "expected error for unknown setting")
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{Pad: 1}
	defaults := r.DefaultSettings()
	pad, ok := defaults["pad"]
	require.True(t, ok, "missing pad in defaults")
	assert.Equal(t, 1, pad, "default pad = %v, want 1", pad)
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

func newTestFile(t *testing.T, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err, "NewFile: %v", err)
	return f
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
