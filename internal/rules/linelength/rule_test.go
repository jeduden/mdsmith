package linelength

import (
	"strings"
	"testing"

	"github.com/jeduden/tidymark/internal/lint"
)

// helper to build a string of n characters.
func nChars(n int, ch byte) string {
	return strings.Repeat(string(ch), n)
}

// defaultExclude returns the default exclude list for convenience.
func defaultExclude() []string {
	return []string{"code-blocks", "tables", "urls"}
}

func TestCheck_LineExceeding80Reports(t *testing.T) {
	// 81-character line should trigger a diagnostic.
	src := []byte(nChars(81, 'a') + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	d := diags[0]
	if d.RuleID != "TM001" {
		t.Errorf("expected rule ID TM001, got %s", d.RuleID)
	}
	if d.RuleName != "line-length" {
		t.Errorf("expected rule name line-length, got %s", d.RuleName)
	}
	if d.Severity != lint.Warning {
		t.Errorf("expected severity warning, got %s", d.Severity)
	}
}

func TestCheck_LineExactly80NoReport(t *testing.T) {
	// Exactly 80 characters should not trigger.
	src := []byte(nChars(80, 'a') + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestCheck_81CharsMessage(t *testing.T) {
	src := []byte(nChars(81, 'x') + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	expected := "line too long (81 > 80)"
	if diags[0].Message != expected {
		t.Errorf("expected message %q, got %q", expected, diags[0].Message)
	}
}

func TestCheck_CustomMax120(t *testing.T) {
	// 100-char line should not trigger with max=120.
	src := []byte(nChars(100, 'a') + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 120, Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for 100-char line with max=120, got %d", len(diags))
	}
}

func TestCheck_CustomMax120_Exceeds(t *testing.T) {
	// 121-char line should trigger with max=120.
	src := []byte(nChars(121, 'a') + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 120, Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	expected := "line too long (121 > 120)"
	if diags[0].Message != expected {
		t.Errorf("expected message %q, got %q", expected, diags[0].Message)
	}
}

func TestCheck_ExcludeCodeBlocks_FencedCodeBlockSkipped(t *testing.T) {
	long := nChars(100, 'x')
	src := []byte("# Heading\n\n```\n" + long + "\n```\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for long line inside fenced code block, got %d", len(diags))
	}
}

func TestCheck_ExcludeCodeBlocks_IndentedCodeBlockSkipped(t *testing.T) {
	// Indented code block: 4 spaces of indentation.
	long := "    " + nChars(100, 'x')
	src := []byte("Some paragraph.\n\n" + long + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for long line inside indented code block, got %d", len(diags))
	}
}

func TestCheck_ExcludeURLs_URLOnlyLineSkipped(t *testing.T) {
	longURL := "https://very-long-url.example.com/" + nChars(80, 'a')
	src := []byte(longURL + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for URL-only long line, got %d", len(diags))
	}
}

func TestCheck_ExcludeURLs_URLOnlyWithLeadingWhitespace(t *testing.T) {
	longURL := "  https://very-long-url.example.com/" + nChars(80, 'b')
	src := []byte(longURL + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for URL-only line with whitespace, got %d", len(diags))
	}
}

func TestCheck_ExcludeURLs_URLWithTextNotSkipped(t *testing.T) {
	// A line that has a URL plus other text should NOT be skipped.
	longLine := "See https://example.com/" + nChars(80, 'c') + " for details"
	src := []byte(longLine + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic for line with URL plus text, got %d", len(diags))
	}
}

func TestCheck_EmptyExclude_FencedCodeBlockFlagged(t *testing.T) {
	long := nChars(100, 'x')
	src := []byte("# Heading\n\n```\n" + long + "\n```\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Exclude: []string{}}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic for long line inside fenced code block (exclude=[]), got %d", len(diags))
	}
}

func TestCheck_EmptyExclude_IndentedCodeBlockFlagged(t *testing.T) {
	long := "    " + nChars(100, 'x')
	src := []byte("Some paragraph.\n\n" + long + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Exclude: []string{}}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic for long line inside indented code block (exclude=[]), got %d", len(diags))
	}
}

func TestCheck_EmptyExclude_URLOnlyLineFlagged(t *testing.T) {
	longURL := "https://very-long-url.example.com/" + nChars(80, 'a')
	src := []byte(longURL + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Exclude: []string{}}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic for URL-only line (exclude=[]), got %d", len(diags))
	}
}

func TestCheck_DiagnosticColumn(t *testing.T) {
	// Column should be max + 1.
	src := []byte(nChars(90, 'z') + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].Column != 81 {
		t.Errorf("expected column 81, got %d", diags[0].Column)
	}
}

func TestCheck_DiagnosticColumnWithCustomMax(t *testing.T) {
	src := []byte(nChars(130, 'z') + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 120, Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].Column != 121 {
		t.Errorf("expected column 121, got %d", diags[0].Column)
	}
}

func TestCheck_MultipleLongLines(t *testing.T) {
	line1 := nChars(90, 'a')
	line2 := nChars(50, 'b') // short
	line3 := nChars(100, 'c')
	src := []byte(line1 + "\n" + line2 + "\n" + line3 + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnostics, got %d", len(diags))
	}
	if diags[0].Line != 1 {
		t.Errorf("expected first diagnostic on line 1, got %d", diags[0].Line)
	}
	if diags[1].Line != 3 {
		t.Errorf("expected second diagnostic on line 3, got %d", diags[1].Line)
	}
}

func TestCheck_EmptyFile(t *testing.T) {
	src := []byte("")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for empty file, got %d", len(diags))
	}
}

func TestCheck_DefaultsEnabledWithMax80(t *testing.T) {
	// Verify the init-registered rule has correct defaults.
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	if r.Max != 80 {
		t.Errorf("expected default max 80, got %d", r.Max)
	}
	if r.ID() != "TM001" {
		t.Errorf("expected ID TM001, got %s", r.ID())
	}
	if r.Name() != "line-length" {
		t.Errorf("expected name line-length, got %s", r.Name())
	}
}

func TestCheck_FencedCodeBlockWithInfoString(t *testing.T) {
	long := nChars(100, 'x')
	src := []byte("# Heading\n\n```go\n" + long + "\n```\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for long line inside fenced code block with info string, got %d", len(diags))
	}
}

func TestCheck_FencedCodeBlockFenceLinesAlsoSkipped(t *testing.T) {
	// Even if the opening/closing fence lines themselves were very long
	// (unlikely but possible), they should be skipped when code-blocks excluded.
	longInfo := "```" + nChars(100, 'x')
	src := []byte("# Title\n\n" + longInfo + "\ncode\n```\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for long fence opening line, got %d", len(diags))
	}
}

func TestCheck_HttpURLSkipped(t *testing.T) {
	longURL := "http://example.com/" + nChars(80, 'a')
	src := []byte(longURL + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for http URL-only long line, got %d", len(diags))
	}
}

func TestCheck_MixedContentWithCodeBlockAndLongLines(t *testing.T) {
	// Long line outside code block should still be flagged.
	longOutside := nChars(90, 'o')
	longInside := nChars(100, 'i')
	src := []byte(longOutside + "\n\n```\n" + longInside + "\n```\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic (only the line outside code block), got %d", len(diags))
	}
	if diags[0].Line != 1 {
		t.Errorf("expected diagnostic on line 1, got %d", diags[0].Line)
	}
}

func TestCheck_TildeFencedCodeBlockSkipped(t *testing.T) {
	long := nChars(100, 'x')
	src := []byte("# Heading\n\n~~~\n" + long + "\n~~~\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for long line inside tilde fenced code block, got %d", len(diags))
	}
}

func TestCheck_MultipleFencedCodeBlocks(t *testing.T) {
	long := nChars(100, 'x')
	src := []byte("```\n" + long + "\n```\n\nshort\n\n```\n" + long + "\n```\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for long lines inside multiple fenced code blocks, got %d", len(diags))
	}
}

func TestCheck_DiagnosticFile(t *testing.T) {
	src := []byte(nChars(81, 'a') + "\n")
	f, err := lint.NewFile("myfile.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].File != "myfile.md" {
		t.Errorf("expected file myfile.md, got %s", diags[0].File)
	}
}

func TestCheck_EmptyFencedCodeBlock(t *testing.T) {
	src := []byte("```\n```\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for empty fenced code block, got %d", len(diags))
	}
}

func TestCheck_LongLineOnLastLineNoTrailingNewline(t *testing.T) {
	src := []byte(nChars(81, 'a'))
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].Line != 1 {
		t.Errorf("expected line 1, got %d", diags[0].Line)
	}
}

// --- Configurable tests ---

func TestApplySettings_ValidMax(t *testing.T) {
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	err := r.ApplySettings(map[string]any{"max": 120})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Max != 120 {
		t.Errorf("expected Max=120, got %d", r.Max)
	}
}

func TestApplySettings_InvalidMaxType(t *testing.T) {
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	err := r.ApplySettings(map[string]any{"max": "not-a-number"})
	if err == nil {
		t.Fatal("expected error for non-int max")
	}
}

func TestApplySettings_ValidExclude(t *testing.T) {
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	err := r.ApplySettings(map[string]any{"exclude": []any{"code-blocks", "urls"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.Exclude) != 2 || r.Exclude[0] != "code-blocks" || r.Exclude[1] != "urls" {
		t.Errorf("unexpected Exclude: %v", r.Exclude)
	}
}

func TestApplySettings_EmptyExclude(t *testing.T) {
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	err := r.ApplySettings(map[string]any{"exclude": []any{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.Exclude) != 0 {
		t.Errorf("expected empty Exclude, got %v", r.Exclude)
	}
}

func TestApplySettings_InvalidExcludeValue(t *testing.T) {
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	err := r.ApplySettings(map[string]any{"exclude": []any{"invalid"}})
	if err == nil {
		t.Fatal("expected error for invalid exclude value")
	}
}

func TestApplySettings_InvalidExcludeType(t *testing.T) {
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	err := r.ApplySettings(map[string]any{"exclude": 42})
	if err == nil {
		t.Fatal("expected error for non-list exclude")
	}
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	err := r.ApplySettings(map[string]any{"unknown": true})
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
}

func TestApplySettings_StrictTrueDeprecationShim(t *testing.T) {
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	err := r.ApplySettings(map[string]any{"strict": true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.Exclude) != 0 {
		t.Errorf("expected empty Exclude for strict:true, got %v", r.Exclude)
	}
}

func TestApplySettings_StrictFalseDeprecationShim(t *testing.T) {
	r := &Rule{Max: 80, Exclude: []string{}}
	err := r.ApplySettings(map[string]any{"strict": false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.Exclude) != 3 {
		t.Errorf("expected 3 exclude items for strict:false, got %v", r.Exclude)
	}
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	if ds["max"] != 80 {
		t.Errorf("expected max=80, got %v", ds["max"])
	}
	exclude, ok := ds["exclude"].([]string)
	if !ok {
		t.Fatalf("expected exclude to be []string, got %T", ds["exclude"])
	}
	if len(exclude) != 3 {
		t.Errorf("expected 3 exclude items, got %d", len(exclude))
	}
}

// --- Table exclusion tests ---

func TestCheck_ExcludeTables_LongTableRowSkipped(t *testing.T) {
	longRow := "| " + nChars(100, 'x') + " | " + nChars(100, 'y') + " |"
	src := []byte("# Title\n\n" + longRow + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Exclude: []string{"tables"}}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for long table row with tables excluded, got %d", len(diags))
	}
}

func TestCheck_NoExcludeTables_LongTableRowFlagged(t *testing.T) {
	longRow := "| " + nChars(100, 'x') + " | " + nChars(100, 'y') + " |"
	src := []byte("# Title\n\n" + longRow + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Exclude: []string{"code-blocks", "urls"}}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic for long table row without tables excluded, got %d", len(diags))
	}
}

func TestCheck_ExcludeOnlyCodeBlocks(t *testing.T) {
	// Only code-blocks excluded: tables and URLs should be flagged.
	longURL := "https://example.com/" + nChars(80, 'a')
	longRow := "| " + nChars(100, 'x') + " |"
	long := nChars(100, 'z')
	src := []byte("```\n" + long + "\n```\n\n" + longURL + "\n\n" + longRow + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Exclude: []string{"code-blocks"}}
	diags := r.Check(f)
	// URL line and table row should both be flagged.
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnostics (URL + table), got %d", len(diags))
	}
}

// --- heading-max tests ---

func intPtr(n int) *int { return &n }

func TestCheck_HeadingMax_HeadingWithinHeadingMaxButOverMax(t *testing.T) {
	// Heading is 70 chars, over max=60 but within heading-max=80: pass.
	heading := "# " + nChars(68, 'h')
	src := []byte(heading + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 60, HeadingMax: intPtr(80), Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for heading within heading-max, got %d", len(diags))
	}
}

func TestCheck_HeadingMax_HeadingOverHeadingMax(t *testing.T) {
	// Heading is 65 chars, over heading-max=60: fail.
	heading := "# " + nChars(63, 'h')
	src := []byte(heading + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, HeadingMax: intPtr(60), Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic for heading over heading-max, got %d", len(diags))
	}
	expected := "line too long (65 > 60)"
	if diags[0].Message != expected {
		t.Errorf("expected message %q, got %q", expected, diags[0].Message)
	}
	if diags[0].Column != 61 {
		t.Errorf("expected column 61, got %d", diags[0].Column)
	}
}

func TestCheck_HeadingMax_UnsetInheritsFromMax(t *testing.T) {
	// heading-max unset: heading uses max=80. 81-char heading: fail.
	heading := "# " + nChars(79, 'h')
	src := []byte(heading + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic for heading over max (heading-max unset), got %d", len(diags))
	}
}

func TestCheck_HeadingMax_NonHeadingUsesBaseMax(t *testing.T) {
	// heading-max is 120, but a regular line should still use max=80.
	line := nChars(90, 'x')
	src := []byte("# Short heading\n\n" + line + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, HeadingMax: intPtr(120), Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic for non-heading line over max, got %d", len(diags))
	}
	expected := "line too long (90 > 80)"
	if diags[0].Message != expected {
		t.Errorf("expected message %q, got %q", expected, diags[0].Message)
	}
}

func TestCheck_HeadingMax_Level2Heading(t *testing.T) {
	// heading-max applies to all heading levels.
	heading := "## " + nChars(68, 'h') // 71 chars total
	src := []byte(heading + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 60, HeadingMax: intPtr(80), Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for level-2 heading within heading-max, got %d", len(diags))
	}
}

// --- code-block-max tests ---

func TestCheck_CodeBlockMax_CodeLineWithinCodeBlockMaxButOverMax(t *testing.T) {
	// Code line is 100 chars, over max=80 but within code-block-max=120: pass.
	long := nChars(100, 'c')
	src := []byte("# Title\n\n```\n" + long + "\n```\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, CodeBlockMax: intPtr(120), Exclude: []string{"tables", "urls"}}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for code line within code-block-max, got %d", len(diags))
	}
}

func TestCheck_CodeBlockMax_CodeLineOverCodeBlockMax(t *testing.T) {
	// Code line is 130 chars, over code-block-max=120: fail.
	long := nChars(130, 'c')
	src := []byte("# Title\n\n```\n" + long + "\n```\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, CodeBlockMax: intPtr(120), Exclude: []string{"tables", "urls"}}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic for code line over code-block-max, got %d", len(diags))
	}
	expected := "line too long (130 > 120)"
	if diags[0].Message != expected {
		t.Errorf("expected message %q, got %q", expected, diags[0].Message)
	}
	if diags[0].Column != 121 {
		t.Errorf("expected column 121, got %d", diags[0].Column)
	}
}

func TestCheck_CodeBlockMax_UnsetInheritsFromMax(t *testing.T) {
	// code-block-max unset, code-blocks not excluded: uses max=80.
	long := nChars(90, 'c')
	src := []byte("```\n" + long + "\n```\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Exclude: []string{"tables", "urls"}}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic for code line over max (code-block-max unset), got %d", len(diags))
	}
}

func TestCheck_CodeBlockMax_ExcludeStillSkips(t *testing.T) {
	// code-block-max is set, but exclude includes code-blocks: skip.
	long := nChars(200, 'c')
	src := []byte("```\n" + long + "\n```\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, CodeBlockMax: intPtr(120), Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for code line when code-blocks excluded, got %d", len(diags))
	}
}

func TestCheck_CodeBlockMax_NonCodeLineUsesBaseMax(t *testing.T) {
	// code-block-max=120, but a regular line should still use max=80.
	line := nChars(90, 'x')
	src := []byte(line + "\n\n```\nshort code\n```\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, CodeBlockMax: intPtr(120), Exclude: []string{"tables", "urls"}}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic for non-code line over max, got %d", len(diags))
	}
	expected := "line too long (90 > 80)"
	if diags[0].Message != expected {
		t.Errorf("expected message %q, got %q", expected, diags[0].Message)
	}
}

// --- stern mode tests ---

func TestCheck_Stern_LongLineWithSpacesPastLimit(t *testing.T) {
	// Long line with spaces past limit: flagged in stern mode.
	// 85 'a' chars + space at position 85 + 5 'b' chars = 91 total.
	line := nChars(85, 'a') + " " + nChars(5, 'b')
	src := []byte(line + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Stern: true, Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic for line with spaces past limit in stern mode, got %d", len(diags))
	}
}

func TestCheck_Stern_LongLineNoSpacesPastLimit(t *testing.T) {
	// Long line with no spaces past limit: allowed in stern mode.
	// 75 'a' chars + space at position 75 + 10 'b' chars = 86 total.
	// Space at 75 is before the limit of 80. Chars past 80 are all 'b'.
	line := nChars(75, 'a') + " " + nChars(10, 'b')
	src := []byte(line + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Stern: true, Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for line without spaces past limit in stern mode, got %d", len(diags))
	}
}

func TestCheck_Stern_DisabledStillFlags(t *testing.T) {
	// Same line, stern=false: should be flagged.
	line := nChars(75, 'a') + " " + nChars(10, 'b')
	src := []byte(line + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Stern: false, Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic with stern disabled, got %d", len(diags))
	}
}

func TestCheck_Stern_ExcludeInteraction(t *testing.T) {
	// Stern mode + exclude: excluded lines stay excluded.
	long := nChars(100, 'x')
	src := []byte("```\n" + long + "\n```\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Stern: true, Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for excluded code line in stern mode, got %d", len(diags))
	}
}

func TestCheck_Stern_WithHeadingMax(t *testing.T) {
	// Stern applies to the active max for heading lines.
	// "# " (2) + 58 'h' chars + space at position 60 + 5 'x' chars = 66 total.
	// heading-max=60, so the space at position 60 is past the limit.
	heading := "# " + nChars(58, 'h') + " " + nChars(5, 'x')
	src := []byte(heading + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, HeadingMax: intPtr(60), Stern: true, Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic for heading with space past heading-max in stern mode, got %d", len(diags))
	}
}

func TestCheck_Stern_WithHeadingMaxNoSpacePast(t *testing.T) {
	// Heading exceeds heading-max=60 but has no space past limit: allowed.
	heading := "# " + nChars(63, 'h') // 65 chars, no spaces past position 60
	src := []byte(heading + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, HeadingMax: intPtr(60), Stern: true, Exclude: defaultExclude()}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for heading without spaces past heading-max, got %d", len(diags))
	}
}

func TestCheck_Stern_WithCodeBlockMax(t *testing.T) {
	// Code line exceeds code-block-max but has space past limit: flagged.
	long := nChars(110, 'c') + " " + nChars(10, 'd')
	src := []byte("```\n" + long + "\n```\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, CodeBlockMax: intPtr(100), Stern: true, Exclude: []string{"tables", "urls"}}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic for code line with space past code-block-max, got %d", len(diags))
	}
}

// --- ApplySettings tests for new settings ---

func TestApplySettings_ValidHeadingMax(t *testing.T) {
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	err := r.ApplySettings(map[string]any{"heading-max": 100})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.HeadingMax == nil || *r.HeadingMax != 100 {
		t.Errorf("expected HeadingMax=100, got %v", r.HeadingMax)
	}
}

func TestApplySettings_InvalidHeadingMaxType(t *testing.T) {
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	err := r.ApplySettings(map[string]any{"heading-max": "not-a-number"})
	if err == nil {
		t.Fatal("expected error for non-int heading-max")
	}
}

func TestApplySettings_HeadingMaxMustBePositive(t *testing.T) {
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	err := r.ApplySettings(map[string]any{"heading-max": 0})
	if err == nil {
		t.Fatal("expected error for zero heading-max")
	}
}

func TestApplySettings_ValidCodeBlockMax(t *testing.T) {
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	err := r.ApplySettings(map[string]any{"code-block-max": 120})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.CodeBlockMax == nil || *r.CodeBlockMax != 120 {
		t.Errorf("expected CodeBlockMax=120, got %v", r.CodeBlockMax)
	}
}

func TestApplySettings_InvalidCodeBlockMaxType(t *testing.T) {
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	err := r.ApplySettings(map[string]any{"code-block-max": "not-a-number"})
	if err == nil {
		t.Fatal("expected error for non-int code-block-max")
	}
}

func TestApplySettings_CodeBlockMaxMustBePositive(t *testing.T) {
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	err := r.ApplySettings(map[string]any{"code-block-max": -5})
	if err == nil {
		t.Fatal("expected error for negative code-block-max")
	}
}

func TestApplySettings_ValidStern(t *testing.T) {
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	err := r.ApplySettings(map[string]any{"stern": true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.Stern {
		t.Errorf("expected Stern=true")
	}
}

func TestApplySettings_InvalidSternType(t *testing.T) {
	r := &Rule{Max: 80, Exclude: defaultExclude()}
	err := r.ApplySettings(map[string]any{"stern": "not-a-bool"})
	if err == nil {
		t.Fatal("expected error for non-bool stern")
	}
}

func TestDefaultSettings_IncludesStern(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	stern, ok := ds["stern"].(bool)
	if !ok {
		t.Fatalf("expected stern to be bool, got %T", ds["stern"])
	}
	if stern {
		t.Errorf("expected stern=false by default")
	}
}
