package fencedcodelanguage

import (
	"testing"

	"github.com/jeduden/tidymark/internal/lint"
)

func TestCheck_MissingLanguage(t *testing.T) {
	src := []byte("```\ncode\n```\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	d := diags[0]
	if d.RuleID != "TM011" {
		t.Errorf("expected rule ID TM011, got %s", d.RuleID)
	}
	if d.Line != 1 {
		t.Errorf("expected line 1, got %d", d.Line)
	}
	if d.Message != "fenced code block should have a language tag" {
		t.Errorf("unexpected message: %s", d.Message)
	}
}

func TestCheck_WithLanguage(t *testing.T) {
	src := []byte("```go\nfmt.Println()\n```\n")
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

func TestCheck_TildeWithLanguage(t *testing.T) {
	src := []byte("~~~python\nprint()\n~~~\n")
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

func TestCheck_TildeWithoutLanguage(t *testing.T) {
	src := []byte("~~~\ncode\n~~~\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
}

func TestCheck_EmptyCodeBlock(t *testing.T) {
	src := []byte("```\n```\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
}

func TestCheck_MultipleBlocks_OnlyMissingFlagged(t *testing.T) {
	src := []byte("```go\ncode1\n```\n\n```\ncode2\n```\n\n```python\ncode3\n```\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].Line != 5 {
		t.Errorf("expected line 5, got %d", diags[0].Line)
	}
}

func TestCheck_DiagnosticPointsToOpeningFence(t *testing.T) {
	src := []byte("# Title\n\n```\ncode\n```\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].Line != 3 {
		t.Errorf("expected line 3, got %d", diags[0].Line)
	}
}

func TestCheck_ID(t *testing.T) {
	r := &Rule{}
	if r.ID() != "TM011" {
		t.Errorf("expected ID TM011, got %s", r.ID())
	}
}

func TestCheck_Name(t *testing.T) {
	r := &Rule{}
	if r.Name() != "fenced-code-language" {
		t.Errorf("expected name fenced-code-language, got %s", r.Name())
	}
}
