package noduplicateheadings

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
)

func TestCheck_NoDuplicates_NoViolation(t *testing.T) {
	src := []byte("# Title\n\n## Section A\n\n## Section B\n")
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

func TestCheck_DuplicateHeadings(t *testing.T) {
	src := []byte("# Title\n\n## Section\n\n## Section\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %+v", len(diags), diags)
	}
	if diags[0].RuleID != "MDS005" {
		t.Errorf("expected rule ID MDS005, got %s", diags[0].RuleID)
	}
}

func TestCheck_DuplicatesDifferentLevels(t *testing.T) {
	src := []byte("# Title\n\n## Title\n")
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

func TestCheck_MultipleDuplicates(t *testing.T) {
	src := []byte("# Title\n\n## Title\n\n### Title\n")
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

func TestCheck_NoHeadings(t *testing.T) {
	src := []byte("Some text.\n")
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

func TestCheck_DuplicateEllipsisAllowed(t *testing.T) {
	src := []byte("# Title\n\n## ...\n\n## ...\n")
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

func TestID(t *testing.T) {
	r := &Rule{}
	if r.ID() != "MDS005" {
		t.Errorf("expected MDS005, got %s", r.ID())
	}
}

func TestName(t *testing.T) {
	r := &Rule{}
	if r.Name() != "no-duplicate-headings" {
		t.Errorf("expected no-duplicate-headings, got %s", r.Name())
	}
}
