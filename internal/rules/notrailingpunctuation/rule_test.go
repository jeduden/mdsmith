package notrailingpunctuation

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
)

func TestCheck_NoPunctuation_NoViolation(t *testing.T) {
	src := []byte("# Title\n\n## Section\n")
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

func TestCheck_TrailingPeriod(t *testing.T) {
	src := []byte("# Title.\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %+v", len(diags), diags)
	}
	if diags[0].RuleID != "MDS017" {
		t.Errorf("expected rule ID MDS017, got %s", diags[0].RuleID)
	}
}

func TestCheck_TrailingComma(t *testing.T) {
	src := []byte("# Title,\n")
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

func TestCheck_TrailingColon(t *testing.T) {
	src := []byte("# Title:\n")
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

func TestCheck_TrailingSemicolon(t *testing.T) {
	src := []byte("# Title;\n")
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

func TestCheck_TrailingExclamation(t *testing.T) {
	src := []byte("# Title!\n")
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

func TestCheck_QuestionMark_NoViolation(t *testing.T) {
	src := []byte("# Is this a title?\n")
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

func TestCheck_EllipsisHeading_NoViolation(t *testing.T) {
	src := []byte("## ...\n")
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
	if r.ID() != "MDS017" {
		t.Errorf("expected MDS017, got %s", r.ID())
	}
}

func TestName(t *testing.T) {
	r := &Rule{}
	if r.Name() != "no-trailing-punctuation-in-heading" {
		t.Errorf("expected no-trailing-punctuation-in-heading, got %s", r.Name())
	}
}
