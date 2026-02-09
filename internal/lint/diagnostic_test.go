package lint

import "testing"

func TestDiagnosticFields(t *testing.T) {
	d := Diagnostic{
		File:     "README.md",
		Line:     10,
		Column:   5,
		RuleID:   "TM001",
		RuleName: "line-length",
		Severity: Error,
		Message:  "line too long (120 > 80)",
	}

	if d.File != "README.md" {
		t.Errorf("expected File %q, got %q", "README.md", d.File)
	}
	if d.Line != 10 {
		t.Errorf("expected Line 10, got %d", d.Line)
	}
	if d.Column != 5 {
		t.Errorf("expected Column 5, got %d", d.Column)
	}
	if d.RuleID != "TM001" {
		t.Errorf("expected RuleID %q, got %q", "TM001", d.RuleID)
	}
	if d.RuleName != "line-length" {
		t.Errorf("expected RuleName %q, got %q", "line-length", d.RuleName)
	}
	if d.Severity != Error {
		t.Errorf("expected Severity %q, got %q", Error, d.Severity)
	}
	if d.Message != "line too long (120 > 80)" {
		t.Errorf("expected Message %q, got %q", "line too long (120 > 80)", d.Message)
	}
}

func TestSeverityConstants(t *testing.T) {
	if Error != "error" {
		t.Errorf("expected Error to be %q, got %q", "error", Error)
	}
	if Warning != "warning" {
		t.Errorf("expected Warning to be %q, got %q", "warning", Warning)
	}
}
