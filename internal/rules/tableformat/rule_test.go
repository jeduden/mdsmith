package tableformat

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rules/tablefmt"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Display-width Fix regression tests ---

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
		"| ------ | ------------------------- |\n" +
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
	assert.Contains(t, diags[0].Message, "table is not formatted",
		"message should contain base description")
	assert.Contains(t, diags[0].Message, "| Name   | Description               |",
		"message should show expected first differing row")
}

func TestCheck_DiagnosticShowsFirstDifferingRow(t *testing.T) {
	// Header row is already correct width, but separator is too short.
	src := "| Name | Desc |\n" +
		"|---|---|\n" +
		"| foo  | bar  |\n"
	r := &Rule{Pad: 1}
	f := newTestFile(t, src)
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	// The first differing row is the separator (row 2).
	assert.Contains(t, diags[0].Message, "row 2",
		"message should reference the first differing row")
	assert.Contains(t, diags[0].Message, "| ---- | ---- |",
		"message should show expected spaced-style separator")
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
		"| ------ | ------------------------- |\n" +
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

	// Verify alignment indicators are preserved at the spaced positions.
	lines := strings.Split(got, "\n")
	sep := lines[1]
	assert.Contains(t, sep, ":", "alignment indicators not preserved in separator: %q", sep)
	// Left alignment: | :--- (colon flush with content)
	assert.Contains(t, sep, "| :", "left alignment not preserved: %q", sep)
	// Right alignment: ---: | (colon flush with content)
	assert.Contains(t, sep, ": |", "right alignment not preserved: %q", sep)
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
	r := &Rule{Pad: 1, Style: StyleConsistent}
	defaults := r.DefaultSettings()
	assert.Equal(t, 1, defaults["pad"], "default pad")
	assert.Equal(t, "spaced", defaults["separator-style"], "default separator-style")
	assert.Equal(t, StyleConsistent, defaults["style"], "default style")
}

// --- separator-style tests ---

func TestApplySettings_SeparatorStyle(t *testing.T) {
	cases := map[string]struct {
		value     any
		wantErr   bool
		wantStyle tablefmt.SeparatorStyle
	}{
		"compact":    {value: "compact", wantStyle: tablefmt.SeparatorCompact},
		"spaced":     {value: "spaced", wantStyle: tablefmt.SeparatorSpaced},
		"invalid":    {value: "wide", wantErr: true},
		"wrong type": {value: 42, wantErr: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := &Rule{Pad: 1}
			err := r.ApplySettings(map[string]any{"separator-style": tc.value})
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantStyle, r.SeparatorStyle)
		})
	}
}

func TestCheck_CompactStyle_FlagsSpacedSeparator(t *testing.T) {
	src := "| Name | Value |\n" +
		"| ---- | ----- |\n" +
		"| foo  | bar   |\n"
	r := &Rule{Pad: 1, SeparatorStyle: tablefmt.SeparatorCompact}
	f := newTestFile(t, src)
	diags := r.Check(f)
	require.Len(t, diags, 1, "spaced separator must be flagged under compact style")
	assert.Contains(t, diags[0].Message, "|------|-------|",
		"expected compact separator to be the suggested fix; got: %s", diags[0].Message)
}

func TestFix_CompactStyle_RewritesSpacedToCompact(t *testing.T) {
	src := "| Name | Value |\n" +
		"| ---- | ----- |\n" +
		"| foo  | bar   |\n"
	r := &Rule{Pad: 1, SeparatorStyle: tablefmt.SeparatorCompact}
	f := newTestFile(t, src)
	fixed := string(r.Fix(f))
	assert.Contains(t, fixed, "|------|-------|",
		"Fix must rewrite spaced separator to compact; got:\n%s", fixed)
}

func TestGetSeparatorStyle(t *testing.T) {
	assert.Equal(t, tablefmt.SeparatorSpaced, (&Rule{}).GetSeparatorStyle())
	r := &Rule{SeparatorStyle: tablefmt.SeparatorCompact}
	assert.Equal(t, tablefmt.SeparatorCompact, r.GetSeparatorStyle())
}

// --- Helper functions ---

func newTestFile(t *testing.T, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err, "NewFile: %v", err)
	return f
}

func TestCategory(t *testing.T) {
	r := &Rule{}
	assert.NotEmpty(t, r.Category())
}

// --- Direct unit pin for (*Rule).Fix ---

// TestFix_DirectlyDrivesFormatLines pins the method body directly:
// it must collect code-block lines from the file and forward to
// tablefmt.FormatLines with the rule's config. Driven on a small
// hand-built source with one non-canonical table; the asserted
// output is the byte-exact canonical layout under the default
// Pad=1 / SeparatorSpaced shape so a regression in either the
// CollectCodeBlockLines wiring or the FormatLines call site
// fails here, not in a downstream integration test.
func TestFix_DirectlyDrivesFormatLines(t *testing.T) {
	r := &Rule{Pad: 1, SeparatorStyle: tablefmt.SeparatorSpaced}
	src := "|a|b|\n|---|---|\n|1|2|\n"
	f, err := lint.NewFile("t.md", []byte(src))
	require.NoError(t, err)
	got := string(r.Fix(f))
	want := "| a   | b   |\n| --- | --- |\n| 1   | 2   |\n"
	if got != want {
		t.Errorf("Fix() = %q, want %q", got, want)
	}
}

// TestFix_SkipsTablesInsideCodeBlock pins the CollectCodeBlockLines
// wiring: a table-shaped block inside a fenced code block must NOT
// be rewritten. If Fix dropped the codeLines argument to
// FormatLines, this test would fail because the inner pipe-text
// would be reformatted.
func TestFix_SkipsTablesInsideCodeBlock(t *testing.T) {
	r := &Rule{Pad: 1, SeparatorStyle: tablefmt.SeparatorSpaced}
	src := "```\n|a|b|\n|---|---|\n|1|2|\n```\n"
	f, err := lint.NewFile("t.md", []byte(src))
	require.NoError(t, err)
	got := string(r.Fix(f))
	// Fenced content must pass through byte-for-byte.
	if got != src {
		t.Errorf("Fix() rewrote content inside fenced block\n  got:  %q\n  want: %q",
			got, src)
	}
}

// TestFix_RespectsSeparatorStyleConfig pins that r.config() forwards
// the rule's SeparatorStyle field through to FormatLines. Without
// this, a regression that hard-coded the style at the call site
// would only show in compact-style fixtures.
func TestFix_RespectsSeparatorStyleConfig(t *testing.T) {
	r := &Rule{Pad: 1, SeparatorStyle: tablefmt.SeparatorCompact}
	src := "| a | b |\n| --- | --- |\n| 1 | 2 |\n"
	f, err := lint.NewFile("t.md", []byte(src))
	require.NoError(t, err)
	got := string(r.Fix(f))
	// Compact style fills the cell area with dashes, absorbing
	// the pad spaces.
	if !strings.Contains(got, "|-----|-----|") {
		t.Errorf("Fix() with SeparatorCompact did not produce compact "+
			"separator row\n  got: %q", got)
	}
}
