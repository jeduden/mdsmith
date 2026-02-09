package headingincrement

import (
	"testing"

	"github.com/jeduden/tidymark/internal/lint"
)

func TestCheck_ProperIncrement_NoViolation(t *testing.T) {
	src := []byte("# H1\n\n## H2\n\n### H3\n")
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

func TestCheck_SkipsLevel(t *testing.T) {
	src := []byte("# H1\n\n### H3\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %+v", len(diags), diags)
	}
	if diags[0].RuleID != "TM003" {
		t.Errorf("expected rule ID TM003, got %s", diags[0].RuleID)
	}
}

func TestCheck_FirstHeadingH2_SkipsH1(t *testing.T) {
	src := []byte("## H2 as first heading\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %+v", len(diags), diags)
	}
	if diags[0].Message != "first heading level should be 1, got 2" {
		t.Errorf("unexpected message: %s", diags[0].Message)
	}
}

func TestCheck_DecreasingLevels_NoViolation(t *testing.T) {
	// Going from h3 back to h2 is fine
	src := []byte("# H1\n\n## H2\n\n### H3\n\n## H2 again\n")
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
	src := []byte("Some text without headings.\n")
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
	if r.ID() != "TM003" {
		t.Errorf("expected TM003, got %s", r.ID())
	}
}

func TestName(t *testing.T) {
	r := &Rule{}
	if r.Name() != "heading-increment" {
		t.Errorf("expected heading-increment, got %s", r.Name())
	}
}
