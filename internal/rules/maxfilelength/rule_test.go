package maxfilelength

import (
	"strings"
	"testing"

	"github.com/jeduden/tidymark/internal/lint"
)

// nLines builds a string of n lines, each with some content,
// ending with a trailing newline.
func nLines(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString("line\n")
	}
	return b.String()
}

func TestCheck_ExactlyMaxLines_NoDiagnostic(t *testing.T) {
	src := []byte(nLines(300))
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 300}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestCheck_MaxPlusOne_Diagnostic(t *testing.T) {
	src := []byte(nLines(301))
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 300}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	d := diags[0]
	if d.RuleID != "TM022" {
		t.Errorf("expected rule ID TM022, got %s", d.RuleID)
	}
	if d.RuleName != "max-file-length" {
		t.Errorf(
			"expected rule name max-file-length, got %s",
			d.RuleName,
		)
	}
	if d.Severity != lint.Warning {
		t.Errorf("expected severity warning, got %s", d.Severity)
	}
	if d.Line != 1 {
		t.Errorf("expected line 1, got %d", d.Line)
	}
	if d.Column != 1 {
		t.Errorf("expected column 1, got %d", d.Column)
	}
	expected := "file too long (301 > 300)"
	if d.Message != expected {
		t.Errorf(
			"expected message %q, got %q", expected, d.Message,
		)
	}
}

func TestCheck_CustomMax_Respected(t *testing.T) {
	src := []byte(nLines(11))
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 10}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	expected := "file too long (11 > 10)"
	if diags[0].Message != expected {
		t.Errorf(
			"expected message %q, got %q", expected, diags[0].Message,
		)
	}
}

func TestCheck_CustomMax_AtLimit_NoDiagnostic(t *testing.T) {
	src := []byte(nLines(10))
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 10}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestCheck_EmptyFile_NoDiagnostic(t *testing.T) {
	src := []byte("")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 300}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestCheck_SingleLine_NoDiagnostic(t *testing.T) {
	src := []byte("# Hello\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 300}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestCheck_NoTrailingNewline(t *testing.T) {
	// 5 lines without trailing newline.
	src := []byte("a\nb\nc\nd\ne")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 5}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestCheck_NoTrailingNewline_Exceeds(t *testing.T) {
	// 6 lines without trailing newline, max 5.
	src := []byte("a\nb\nc\nd\ne\nf")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 5}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	expected := "file too long (6 > 5)"
	if diags[0].Message != expected {
		t.Errorf(
			"expected message %q, got %q", expected, diags[0].Message,
		)
	}
}

func TestCheck_DiagnosticFile(t *testing.T) {
	src := []byte(nLines(301))
	f, err := lint.NewFile("long.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Max: 300}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].File != "long.md" {
		t.Errorf(
			"expected file long.md, got %s", diags[0].File,
		)
	}
}

func TestApplySettings_ValidMax(t *testing.T) {
	r := &Rule{Max: 300}
	err := r.ApplySettings(map[string]any{"max": 500})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Max != 500 {
		t.Errorf("expected Max=500, got %d", r.Max)
	}
}

func TestApplySettings_Float64Max(t *testing.T) {
	r := &Rule{Max: 300}
	err := r.ApplySettings(map[string]any{"max": float64(200)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Max != 200 {
		t.Errorf("expected Max=200, got %d", r.Max)
	}
}

func TestApplySettings_InvalidMaxType(t *testing.T) {
	r := &Rule{Max: 300}
	err := r.ApplySettings(map[string]any{"max": "not-a-number"})
	if err == nil {
		t.Fatal("expected error for non-int max")
	}
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{Max: 300}
	err := r.ApplySettings(map[string]any{"unknown": true})
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	if ds["max"] != 300 {
		t.Errorf("expected max=300, got %v", ds["max"])
	}
}

func TestID(t *testing.T) {
	r := &Rule{}
	if r.ID() != "TM022" {
		t.Errorf("expected ID TM022, got %s", r.ID())
	}
}

func TestName(t *testing.T) {
	r := &Rule{}
	if r.Name() != "max-file-length" {
		t.Errorf(
			"expected name max-file-length, got %s", r.Name(),
		)
	}
}

func TestCategory(t *testing.T) {
	r := &Rule{}
	if r.Category() != "meta" {
		t.Errorf(
			"expected category meta, got %s", r.Category(),
		)
	}
}
