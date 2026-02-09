package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jeduden/tidymark/internal/lint"
)

func TestTextFormatter_SingleDiagnostic(t *testing.T) {
	f := &TextFormatter{Color: false}
	var buf bytes.Buffer

	diagnostics := []lint.Diagnostic{
		{
			File:     "README.md",
			Line:     10,
			Column:   5,
			RuleID:   "TM001",
			RuleName: "line-length",
			Severity: lint.Error,
			Message:  "line too long (120 > 80)",
		},
	}

	err := f.Format(&buf, diagnostics)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "README.md:10:5 TM001 line too long (120 > 80)\n"
	if buf.String() != expected {
		t.Errorf("got %q, want %q", buf.String(), expected)
	}
}

func TestTextFormatter_MultipleDiagnostics(t *testing.T) {
	f := &TextFormatter{Color: false}
	var buf bytes.Buffer

	diagnostics := []lint.Diagnostic{
		{
			File:     "README.md",
			Line:     10,
			Column:   5,
			RuleID:   "TM001",
			RuleName: "line-length",
			Severity: lint.Error,
			Message:  "line too long (120 > 80)",
		},
		{
			File:     "docs/guide.md",
			Line:     3,
			Column:   1,
			RuleID:   "TM002",
			RuleName: "first-heading",
			Severity: lint.Warning,
			Message:  "first line should be a heading",
		},
	}

	err := f.Format(&buf, diagnostics)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), buf.String())
	}

	expected1 := "README.md:10:5 TM001 line too long (120 > 80)"
	expected2 := "docs/guide.md:3:1 TM002 first line should be a heading"

	if lines[0] != expected1 {
		t.Errorf("line 1: got %q, want %q", lines[0], expected1)
	}
	if lines[1] != expected2 {
		t.Errorf("line 2: got %q, want %q", lines[1], expected2)
	}
}

func TestTextFormatter_WithColor(t *testing.T) {
	f := &TextFormatter{Color: true}
	var buf bytes.Buffer

	diagnostics := []lint.Diagnostic{
		{
			File:     "README.md",
			Line:     10,
			Column:   5,
			RuleID:   "TM001",
			RuleName: "line-length",
			Severity: lint.Error,
			Message:  "line too long (120 > 80)",
		},
	}

	err := f.Format(&buf, diagnostics)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()

	// Verify ANSI escape sequences are present
	if !strings.Contains(output, "\033[36m") {
		t.Error("expected cyan ANSI escape sequence (\\033[36m) in output")
	}
	if !strings.Contains(output, "\033[33m") {
		t.Error("expected yellow ANSI escape sequence (\\033[33m) in output")
	}
	if !strings.Contains(output, "\033[0m") {
		t.Error("expected reset ANSI escape sequence (\\033[0m) in output")
	}

	// Verify exact colored output
	expected := "\033[36mREADME.md:10:5\033[0m \033[33mTM001\033[0m line too long (120 > 80)\n"
	if output != expected {
		t.Errorf("got %q, want %q", output, expected)
	}
}

func TestTextFormatter_WithoutColor(t *testing.T) {
	f := &TextFormatter{Color: false}
	var buf bytes.Buffer

	diagnostics := []lint.Diagnostic{
		{
			File:     "README.md",
			Line:     10,
			Column:   5,
			RuleID:   "TM001",
			RuleName: "line-length",
			Severity: lint.Error,
			Message:  "line too long (120 > 80)",
		},
	}

	err := f.Format(&buf, diagnostics)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()

	// Verify no ANSI escape sequences
	if strings.Contains(output, "\033[") {
		t.Error("expected no ANSI escape sequences in output, but found some")
	}

	expected := "README.md:10:5 TM001 line too long (120 > 80)\n"
	if output != expected {
		t.Errorf("got %q, want %q", output, expected)
	}
}

func TestTextFormatter_EmptyDiagnostics(t *testing.T) {
	f := &TextFormatter{Color: false}
	var buf bytes.Buffer

	err := f.Format(&buf, []lint.Diagnostic{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if buf.String() != "" {
		t.Errorf("expected empty output for no diagnostics, got %q", buf.String())
	}
}

func TestTextFormatter_ImplementsFormatter(t *testing.T) {
	var _ Formatter = &TextFormatter{}
}
