package tableformat

import (
	"strings"
	"testing"

	"github.com/jeduden/tidymark/internal/lint"
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
		if got != tt.want {
			t.Errorf("displayWidth(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestDisplayWidth_Multibyte(t *testing.T) {
	// Each rune counts as 1 for basic multibyte.
	got := displayWidth("café")
	if got != 4 {
		t.Errorf("displayWidth(café) = %d, want 4", got)
	}
}

func TestDisplayWidth_Link(t *testing.T) {
	got := displayWidth("[text](https://example.com)")
	if got != 4 { // only "text" is visible
		t.Errorf("displayWidth([text](url)) = %d, want 4", got)
	}
}

func TestDisplayWidth_InlineCode(t *testing.T) {
	got := displayWidth("`code`")
	if got != 4 { // only "code" is visible
		t.Errorf("displayWidth(`code`) = %d, want 4", got)
	}
}

func TestDisplayWidth_Bold(t *testing.T) {
	got := displayWidth("**bold**")
	if got != 4 { // only "bold" is visible
		t.Errorf("displayWidth(**bold**) = %d, want 4", got)
	}
}

func TestDisplayWidth_Italic(t *testing.T) {
	got := displayWidth("*italic*")
	if got != 6 { // only "italic" is visible
		t.Errorf("displayWidth(*italic*) = %d, want 6", got)
	}
}

func TestDisplayWidth_Image(t *testing.T) {
	got := displayWidth("![alt text](image.png)")
	if got != 8 { // only "alt text" is visible
		t.Errorf("displayWidth(![alt](url)) = %d, want 8", got)
	}
}

func TestDisplayWidth_Strikethrough(t *testing.T) {
	got := displayWidth("~~deleted~~")
	if got != 7 { // only "deleted" is visible
		t.Errorf("displayWidth(~~deleted~~) = %d, want 7", got)
	}
}

func TestDisplayWidth_Mixed(t *testing.T) {
	// "see text for details" -> "see " + "text" + " for details" = 20
	got := displayWidth("see [text](url) for details")
	if got != 20 {
		t.Errorf("displayWidth(mixed) = %d, want 20", got)
	}
}

// --- Table detection tests ---

func TestFindTables_Basic(t *testing.T) {
	src := "| a | b |\n|---|---|\n| 1 | 2 |\n"
	lines := splitLines(src)
	tables := findTables(lines, nil)
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	if tables[0].startLine != 1 {
		t.Errorf("start line = %d, want 1", tables[0].startLine)
	}
	if len(tables[0].rows) != 3 {
		t.Errorf("rows = %d, want 3", len(tables[0].rows))
	}
}

func TestFindTables_NoTable(t *testing.T) {
	src := "# Heading\n\nSome text.\n"
	lines := splitLines(src)
	tables := findTables(lines, nil)
	if len(tables) != 0 {
		t.Errorf("expected 0 tables, got %d", len(tables))
	}
}

func TestFindTables_TwoTables(t *testing.T) {
	src := "| a | b |\n|---|---|\n| 1 | 2 |\n\n| x | y |\n|---|---|\n| 3 | 4 |\n"
	lines := splitLines(src)
	tables := findTables(lines, nil)
	if len(tables) != 2 {
		t.Fatalf("expected 2 tables, got %d", len(tables))
	}
}

func TestFindTables_SingleColumn(t *testing.T) {
	src := "| a |\n|---|\n| 1 |\n"
	lines := splitLines(src)
	tables := findTables(lines, nil)
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	if len(tables[0].rows[0].cells) != 1 {
		t.Errorf("columns = %d, want 1", len(tables[0].rows[0].cells))
	}
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
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
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
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
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
	if got != want {
		t.Errorf("Fix:\ngot:\n%s\nwant:\n%s", got, want)
	}
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
	if !strings.Contains(sep, ":") {
		t.Errorf("alignment indicators not preserved in separator: %q", sep)
	}
	// Left alignment: |:---
	if !strings.Contains(sep, "|:") {
		t.Errorf("left alignment not preserved: %q", sep)
	}
	// Right alignment: ---:|
	if !strings.Contains(sep, ":|") {
		t.Errorf("right alignment not preserved: %q", sep)
	}
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
	if !strings.Contains(got, `a \| b`) {
		t.Errorf("escaped pipe not preserved:\n%s", got)
	}
}

func TestFix_SingleColumn(t *testing.T) {
	src := "| a |\n|---|\n| b |\n"
	r := &Rule{Pad: 1}
	f := newTestFile(t, src)
	got := string(r.Fix(f))
	// Single column should be properly formatted.
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Pad != 2 {
		t.Errorf("pad = %d, want 2", r.Pad)
	}
}

func TestApplySettings_InvalidPad(t *testing.T) {
	r := &Rule{Pad: 1}
	err := r.ApplySettings(map[string]any{"pad": "two"})
	if err == nil {
		t.Fatal("expected error for non-integer pad")
	}
}

func TestApplySettings_NegativePad(t *testing.T) {
	r := &Rule{Pad: 1}
	err := r.ApplySettings(map[string]any{"pad": -1})
	if err == nil {
		t.Fatal("expected error for negative pad")
	}
}

func TestApplySettings_UnknownSetting(t *testing.T) {
	r := &Rule{Pad: 1}
	err := r.ApplySettings(map[string]any{"unknown": true})
	if err == nil {
		t.Fatal("expected error for unknown setting")
	}
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{Pad: 1}
	defaults := r.DefaultSettings()
	pad, ok := defaults["pad"]
	if !ok {
		t.Fatal("missing pad in defaults")
	}
	if pad != 1 {
		t.Errorf("default pad = %v, want 1", pad)
	}
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
		if got != tt.want {
			t.Errorf("isSeparatorRow(%v) = %v, want %v", tt.cells, got, tt.want)
		}
	}
}

func TestParseAlignments(t *testing.T) {
	cells := []string{"---", ":---", "---:", ":---:"}
	aligns := parseAlignments(cells)
	want := []align{alignNone, alignLeft, alignRight, alignCenter}
	if len(aligns) != len(want) {
		t.Fatalf("len = %d, want %d", len(aligns), len(want))
	}
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
	if err != nil {
		t.Fatalf("NewFile: %v", err)
	}
	return f
}

func assertCells(t *testing.T, want, got []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("cells: got %d, want %d\n  got:  %v\n  want: %v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("cell[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
