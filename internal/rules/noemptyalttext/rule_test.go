package noemptyalttext

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
)

func TestCheck_EmptyAlt_Violation(t *testing.T) {
	src := []byte("# Title\n\n![](image.png)\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %+v", len(diags), diags)
	}
	if diags[0].RuleID != "MDS031" {
		t.Errorf("expected rule ID MDS031, got %s", diags[0].RuleID)
	}
	if diags[0].Severity != lint.Warning {
		t.Errorf("expected warning severity, got %s", diags[0].Severity)
	}
	if diags[0].Line != 3 {
		t.Errorf("expected line 3, got %d", diags[0].Line)
	}
}

func TestCheck_WhitespaceAlt_Violation(t *testing.T) {
	src := []byte("# Title\n\n![  ](image.png)\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %+v", len(diags), diags)
	}
}

func TestCheck_WithAlt_NoViolation(t *testing.T) {
	src := []byte("# Title\n\n![A sunset over the ocean](image.png)\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d: %+v", len(diags), diags)
	}
}

func TestCheck_MultipleImages_MixedViolations(t *testing.T) {
	src := []byte("# Title\n\n![](a.png)\n\n![Good alt](b.png)\n\n![](c.png)\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	diags := r.Check(f)
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnostics, got %d: %+v", len(diags), diags)
	}
}

func TestCheck_NoImages_NoViolation(t *testing.T) {
	src := []byte("# Title\n\nJust text.\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestCheck_EmptyFile(t *testing.T) {
	src := []byte("")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestID(t *testing.T) {
	r := &Rule{}
	if r.ID() != "MDS031" {
		t.Errorf("expected MDS031, got %s", r.ID())
	}
}

func TestName(t *testing.T) {
	r := &Rule{}
	if r.Name() != "no-empty-alt-text" {
		t.Errorf("expected no-empty-alt-text, got %s", r.Name())
	}
}
