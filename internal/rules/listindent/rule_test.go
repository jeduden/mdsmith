package listindent

import (
	"testing"

	"github.com/jeduden/tidymark/internal/lint"
)

func TestCheck_CorrectIndent2Spaces(t *testing.T) {
	src := []byte("- item 1\n  - nested\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Spaces: 2}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d: %+v", len(diags), diags)
	}
}

func TestCheck_WrongIndent4SpacesWhenExpecting2(t *testing.T) {
	src := []byte("- item 1\n    - nested\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Spaces: 2}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %+v", len(diags), diags)
	}
	if diags[0].RuleID != "TM016" {
		t.Errorf("expected rule ID TM016, got %s", diags[0].RuleID)
	}
}

func TestCheck_CorrectIndent4Spaces(t *testing.T) {
	src := []byte("- item 1\n    - nested\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Spaces: 4}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d: %+v", len(diags), diags)
	}
}

func TestCheck_DeeplyNested(t *testing.T) {
	src := []byte("- level 0\n  - level 1\n    - level 2\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Spaces: 2}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for correctly indented deep nesting, got %d: %+v", len(diags), diags)
	}
}

func TestCheck_OrderedList(t *testing.T) {
	src := []byte("1. item 1\n   1. nested\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Spaces: 3}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for correctly indented ordered list, got %d: %+v", len(diags), diags)
	}
}

func TestCheck_EmptyFile(t *testing.T) {
	src := []byte("")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Spaces: 2}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestCheck_FlatList(t *testing.T) {
	src := []byte("- item 1\n- item 2\n- item 3\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Spaces: 2}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for flat list, got %d: %+v", len(diags), diags)
	}
}

func TestFix_AdjustsIndentation(t *testing.T) {
	src := []byte("- item 1\n    - nested\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Spaces: 2}
	result := r.Fix(f)
	expected := "- item 1\n  - nested\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestFix_NoChange(t *testing.T) {
	src := []byte("- item 1\n  - nested\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Spaces: 2}
	result := r.Fix(f)
	if string(result) != string(src) {
		t.Errorf("expected no change, got %q", string(result))
	}
}
