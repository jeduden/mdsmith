package noemphasisasheading

import (
	"testing"

	"github.com/jeduden/tidymark/internal/lint"
)

func TestCheck_BoldParagraph_Violation(t *testing.T) {
	src := []byte("**Bold text**\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %+v", len(diags), diags)
	}
	if diags[0].RuleID != "TM018" {
		t.Errorf("expected rule ID TM018, got %s", diags[0].RuleID)
	}
	if diags[0].Message != "emphasis used instead of a heading" {
		t.Errorf("unexpected message: %s", diags[0].Message)
	}
}

func TestCheck_ItalicParagraph_Violation(t *testing.T) {
	src := []byte("*Italic text*\n")
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

func TestCheck_InlineEmphasis_NoViolation(t *testing.T) {
	src := []byte("Some **bold** text in a paragraph.\n")
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

func TestCheck_NormalParagraph_NoViolation(t *testing.T) {
	src := []byte("Just normal text.\n")
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

func TestCheck_Heading_NoViolation(t *testing.T) {
	src := []byte("# Real Heading\n")
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
	if r.ID() != "TM018" {
		t.Errorf("expected TM018, got %s", r.ID())
	}
}

func TestName(t *testing.T) {
	r := &Rule{}
	if r.Name() != "no-emphasis-as-heading" {
		t.Errorf("expected no-emphasis-as-heading, got %s", r.Name())
	}
}
