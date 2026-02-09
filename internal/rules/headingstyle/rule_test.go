package headingstyle

import (
	"testing"

	"github.com/jeduden/tidymark/internal/lint"
)

func TestCheck_ATXStyle_NoViolation(t *testing.T) {
	src := []byte("# Heading 1\n\n## Heading 2\n\n### Heading 3\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Style: "atx"}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d: %+v", len(diags), diags)
	}
}

func TestCheck_ATXStyle_SetextViolation(t *testing.T) {
	src := []byte("Heading 1\n=========\n\nSome text\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Style: "atx"}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %+v", len(diags), diags)
	}
	if diags[0].RuleID != "TM002" {
		t.Errorf("expected rule ID TM002, got %s", diags[0].RuleID)
	}
	if diags[0].Line != 1 {
		t.Errorf("expected line 1, got %d", diags[0].Line)
	}
}

func TestCheck_SetextStyle_ATXViolation(t *testing.T) {
	src := []byte("# Heading 1\n\nSome text\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Style: "setext"}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %+v", len(diags), diags)
	}
	if diags[0].Message != "heading style should be setext" {
		t.Errorf("unexpected message: %s", diags[0].Message)
	}
}

func TestCheck_SetextStyle_Level3ATX_NoViolation(t *testing.T) {
	// Setext only supports levels 1-2, so level 3+ ATX is fine
	src := []byte("### Heading 3\n\nSome text\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Style: "setext"}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d: %+v", len(diags), diags)
	}
}

func TestCheck_SetextStyle_NoViolation(t *testing.T) {
	src := []byte("Heading 1\n=========\n\nHeading 2\n---------\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Style: "setext"}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d: %+v", len(diags), diags)
	}
}

func TestFix_SetextToATX(t *testing.T) {
	src := []byte("Heading 1\n=========\n\nSome text\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Style: "atx"}
	result := r.Fix(f)
	expected := "# Heading 1\n\nSome text\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestFix_ATXToSetext(t *testing.T) {
	src := []byte("# Heading 1\n\nSome text\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Style: "setext"}
	result := r.Fix(f)
	expected := "Heading 1\n=========\n\nSome text\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestID(t *testing.T) {
	r := &Rule{}
	if r.ID() != "TM002" {
		t.Errorf("expected TM002, got %s", r.ID())
	}
}

func TestName(t *testing.T) {
	r := &Rule{}
	if r.Name() != "heading-style" {
		t.Errorf("expected heading-style, got %s", r.Name())
	}
}
