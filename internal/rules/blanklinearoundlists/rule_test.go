package blanklinearoundlists

import (
	"testing"

	"github.com/jeduden/tidymark/internal/lint"
)

func TestCheck_NoBlanksBeforeList(t *testing.T) {
	src := []byte("Some text\n- item 1\n- item 2\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	diags := r.Check(f)
	// Should report "list should be preceded by a blank line"
	found := false
	for _, d := range diags {
		if d.Message == "list should be preceded by a blank line" {
			found = true
			if d.RuleID != "TM014" {
				t.Errorf("expected rule ID TM014, got %s", d.RuleID)
			}
		}
	}
	if !found {
		t.Errorf("expected diagnostic about missing blank before list, got %d diags: %+v", len(diags), diags)
	}
}

func TestCheck_NoBlanksAfterList(t *testing.T) {
	// Use a heading after the list which creates a clear block boundary.
	// (Plain text after a list without blank line gets absorbed into the list item by goldmark.)
	src := []byte("- item 1\n- item 2\n# After\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	diags := r.Check(f)
	found := false
	for _, d := range diags {
		if d.Message == "list should be followed by a blank line" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected diagnostic about missing blank after list, got %d diags: %+v", len(diags), diags)
	}
}

func TestCheck_BlanksAroundList(t *testing.T) {
	src := []byte("Some text\n\n- item 1\n- item 2\n\nMore text\n")
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

func TestCheck_ListAtStartOfFile(t *testing.T) {
	src := []byte("- item 1\n- item 2\n\nSome text\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for list at start of file, got %d: %+v", len(diags), diags)
	}
}

func TestCheck_ListAtEndOfFile(t *testing.T) {
	src := []byte("Some text\n\n- item 1\n- item 2\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for list at end of file, got %d: %+v", len(diags), diags)
	}
}

func TestCheck_NestedListsNoFlag(t *testing.T) {
	src := []byte("Some text\n\n- item 1\n  - nested 1\n  - nested 2\n- item 2\n\nMore text\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for nested lists, got %d: %+v", len(diags), diags)
	}
}

func TestCheck_ListAfterHeading(t *testing.T) {
	src := []byte("# Heading\n- item 1\n- item 2\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	diags := r.Check(f)
	found := false
	for _, d := range diags {
		if d.Message == "list should be preceded by a blank line" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected diagnostic about missing blank before list after heading, got %d diags: %+v", len(diags), diags)
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

func TestFix_InsertsBlankBefore(t *testing.T) {
	src := []byte("Some text\n- item 1\n- item 2\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	result := r.Fix(f)
	expected := "Some text\n\n- item 1\n- item 2\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestFix_InsertsBlankAfter(t *testing.T) {
	src := []byte("- item 1\n- item 2\n# After\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	result := r.Fix(f)
	expected := "- item 1\n- item 2\n\n# After\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestFix_NoChange(t *testing.T) {
	src := []byte("Some text\n\n- item 1\n- item 2\n\nMore text\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	result := r.Fix(f)
	if string(result) != string(src) {
		t.Errorf("expected no change, got %q", string(result))
	}
}
