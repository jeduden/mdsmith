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

func TestCheck_LineExceeding80Reports(t *testing.T) {
	// 81-character line should trigger a diagnostic.
	src := []byte(nChars(81, 'a') + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Strict: false}
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
	r := &Rule{Max: 80, Strict: false}
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
	r := &Rule{Max: 80, Strict: false}
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
	r := &Rule{Max: 120, Strict: false}
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
	r := &Rule{Max: 120, Strict: false}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	expected := "line too long (121 > 120)"
	if diags[0].Message != expected {
		t.Errorf("expected message %q, got %q", expected, diags[0].Message)
	}
}

func TestCheck_StrictFalse_FencedCodeBlockSkipped(t *testing.T) {
	long := nChars(100, 'x')
	src := []byte("# Heading\n\n```\n" + long + "\n```\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Strict: false}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for long line inside fenced code block (strict=false), got %d", len(diags))
	}
}

func TestCheck_StrictFalse_IndentedCodeBlockSkipped(t *testing.T) {
	// Indented code block: 4 spaces of indentation.
	long := "    " + nChars(100, 'x')
	src := []byte("Some paragraph.\n\n" + long + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Strict: false}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for long line inside indented code block (strict=false), got %d", len(diags))
	}
}

func TestCheck_StrictFalse_URLOnlyLineSkipped(t *testing.T) {
	longURL := "https://very-long-url.example.com/" + nChars(80, 'a')
	src := []byte(longURL + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Strict: false}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for URL-only long line (strict=false), got %d", len(diags))
	}
}

func TestCheck_StrictFalse_URLOnlyWithLeadingWhitespace(t *testing.T) {
	longURL := "  https://very-long-url.example.com/" + nChars(80, 'b')
	src := []byte(longURL + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Strict: false}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for URL-only line with whitespace (strict=false), got %d", len(diags))
	}
}

func TestCheck_StrictFalse_URLWithTextNotSkipped(t *testing.T) {
	// A line that has a URL plus other text should NOT be skipped.
	longLine := "See https://example.com/" + nChars(80, 'c') + " for details"
	src := []byte(longLine + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Strict: false}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic for line with URL plus text, got %d", len(diags))
	}
}

func TestCheck_StrictTrue_FencedCodeBlockFlagged(t *testing.T) {
	long := nChars(100, 'x')
	src := []byte("# Heading\n\n```\n" + long + "\n```\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Strict: true}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic for long line inside fenced code block (strict=true), got %d", len(diags))
	}
}

func TestCheck_StrictTrue_IndentedCodeBlockFlagged(t *testing.T) {
	long := "    " + nChars(100, 'x')
	src := []byte("Some paragraph.\n\n" + long + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Strict: true}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic for long line inside indented code block (strict=true), got %d", len(diags))
	}
}

func TestCheck_StrictTrue_URLOnlyLineFlagged(t *testing.T) {
	longURL := "https://very-long-url.example.com/" + nChars(80, 'a')
	src := []byte(longURL + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Strict: true}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic for URL-only line (strict=true), got %d", len(diags))
	}
}

func TestCheck_DiagnosticColumn(t *testing.T) {
	// Column should be max + 1.
	src := []byte(nChars(90, 'z') + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Strict: false}
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
	r := &Rule{Max: 120, Strict: false}
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
	r := &Rule{Max: 80, Strict: false}
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
	r := &Rule{Max: 80, Strict: false}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for empty file, got %d", len(diags))
	}
}

func TestCheck_DefaultsEnabledWithMax80StrictFalse(t *testing.T) {
	// Verify the init-registered rule has correct defaults.
	r := &Rule{Max: 80, Strict: false}
	if r.Max != 80 {
		t.Errorf("expected default max 80, got %d", r.Max)
	}
	if r.Strict != false {
		t.Errorf("expected default strict false, got %v", r.Strict)
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
	r := &Rule{Max: 80, Strict: false}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for long line inside fenced code block with info string (strict=false), got %d", len(diags))
	}
}

func TestCheck_FencedCodeBlockFenceLinesAlsoSkipped(t *testing.T) {
	// Even if the opening/closing fence lines themselves were very long
	// (unlikely but possible), they should be skipped when not strict.
	longInfo := "```" + nChars(100, 'x')
	src := []byte("# Title\n\n" + longInfo + "\ncode\n```\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Strict: false}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for long fence opening line (strict=false), got %d", len(diags))
	}
}

func TestCheck_HttpURLSkipped(t *testing.T) {
	longURL := "http://example.com/" + nChars(80, 'a')
	src := []byte(longURL + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 80, Strict: false}
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
	r := &Rule{Max: 80, Strict: false}
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
	r := &Rule{Max: 80, Strict: false}
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
	r := &Rule{Max: 80, Strict: false}
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
	r := &Rule{Max: 80, Strict: false}
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
	r := &Rule{Max: 80, Strict: false}
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
	r := &Rule{Max: 80, Strict: false}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].Line != 1 {
		t.Errorf("expected line 1, got %d", diags[0].Line)
	}
}
