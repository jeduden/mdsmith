package firstlineheading

import (
	"testing"

	"github.com/jeduden/tidymark/internal/lint"
)

func TestCheck_FirstLineH1_NoViolation(t *testing.T) {
	src := []byte("# Title\n\nSome text\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Level: 1}
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
	r := &Rule{Level: 1}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].RuleID != "TM004" {
		t.Errorf("expected rule ID TM004, got %s", diags[0].RuleID)
	}
}

func TestCheck_StartsWithParagraph(t *testing.T) {
	src := []byte("Some text\n\n# Title\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Level: 1}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %+v", len(diags), diags)
	}
}

func TestCheck_BlankLineThenHeading(t *testing.T) {
	src := []byte("\n# Title\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Level: 1}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic for heading not on line 1, got %d: %+v", len(diags), diags)
	}
}

func TestCheck_WrongLevel(t *testing.T) {
	src := []byte("## Title\n\nSome text\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Level: 1}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %+v", len(diags), diags)
	}
}

func TestCheck_Level2Config(t *testing.T) {
	src := []byte("## Title\n\nSome text\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Level: 2}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d: %+v", len(diags), diags)
	}
}

func TestID(t *testing.T) {
	r := &Rule{}
	if r.ID() != "TM004" {
		t.Errorf("expected TM004, got %s", r.ID())
	}
}

func TestName(t *testing.T) {
	r := &Rule{}
	if r.Name() != "first-line-heading" {
		t.Errorf("expected first-line-heading, got %s", r.Name())
	}
}
