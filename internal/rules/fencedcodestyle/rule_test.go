package fencedcodestyle

import (
	"testing"

	"github.com/jeduden/tidymark/internal/lint"
)

func TestCheck_BacktickDefault_NoViolation(t *testing.T) {
	src := []byte("# Hello\n\n```go\nfmt.Println()\n```\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Style: "backtick"}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestCheck_TildeWhenBacktickExpected(t *testing.T) {
	src := []byte("# Hello\n\n~~~go\nfmt.Println()\n~~~\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Style: "backtick"}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	d := diags[0]
	if d.RuleID != "TM010" {
		t.Errorf("expected rule ID TM010, got %s", d.RuleID)
	}
	if d.Line != 3 {
		t.Errorf("expected line 3, got %d", d.Line)
	}
	if d.Message != "fenced code block should use backtick style" {
		t.Errorf("unexpected message: %s", d.Message)
	}
}

func TestCheck_TildeStyle_NoViolation(t *testing.T) {
	src := []byte("~~~python\nprint()\n~~~\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Style: "tilde"}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestCheck_BacktickWhenTildeExpected(t *testing.T) {
	src := []byte("```go\nfmt.Println()\n```\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Style: "tilde"}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	d := diags[0]
	if d.Line != 1 {
		t.Errorf("expected line 1, got %d", d.Line)
	}
	if d.Message != "fenced code block should use tilde style" {
		t.Errorf("unexpected message: %s", d.Message)
	}
}

func TestCheck_EmptyCodeBlock(t *testing.T) {
	src := []byte("```\n```\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Style: "backtick"}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestCheck_EmptyCodeBlockWrongStyle(t *testing.T) {
	src := []byte("~~~\n~~~\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Style: "backtick"}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
}

func TestCheck_FourBackticks(t *testing.T) {
	src := []byte("````\ncode\n````\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Style: "backtick"}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestCheck_MultipleBlocks_MixedStyles(t *testing.T) {
	src := []byte("```go\nfmt.Println()\n```\n\n~~~python\nprint()\n~~~\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Style: "backtick"}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].Line != 5 {
		t.Errorf("expected line 5, got %d", diags[0].Line)
	}
}

func TestFix_TildeToBacktick(t *testing.T) {
	src := []byte("~~~go\nfmt.Println()\n~~~\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Style: "backtick"}
	result := r.Fix(f)
	expected := "```go\nfmt.Println()\n```\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestFix_BacktickToTilde(t *testing.T) {
	src := []byte("```go\nfmt.Println()\n```\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Style: "tilde"}
	result := r.Fix(f)
	expected := "~~~go\nfmt.Println()\n~~~\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestFix_PreservesLanguageTag(t *testing.T) {
	src := []byte("~~~python\nprint()\n~~~\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Style: "backtick"}
	result := r.Fix(f)
	expected := "```python\nprint()\n```\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestFix_PreservesContent(t *testing.T) {
	src := []byte("# Title\n\n~~~go\nline1\nline2\n~~~\n\ntext\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Style: "backtick"}
	result := r.Fix(f)
	expected := "# Title\n\n```go\nline1\nline2\n```\n\ntext\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestFix_NoChangeNeeded(t *testing.T) {
	src := []byte("```go\ncode\n```\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Style: "backtick"}
	result := r.Fix(f)
	if string(result) != string(src) {
		t.Errorf("expected no change, got %q", string(result))
	}
}

func TestFix_FourTildesToFourBackticks(t *testing.T) {
	src := []byte("~~~~go\ncode\n~~~~\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Style: "backtick"}
	result := r.Fix(f)
	expected := "````go\ncode\n````\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestFix_EmptyCodeBlock(t *testing.T) {
	src := []byte("~~~\n~~~\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{Style: "backtick"}
	result := r.Fix(f)
	expected := "```\n```\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestCheck_ID(t *testing.T) {
	r := &Rule{Style: "backtick"}
	if r.ID() != "TM010" {
		t.Errorf("expected ID TM010, got %s", r.ID())
	}
}

func TestCheck_Name(t *testing.T) {
	r := &Rule{Style: "backtick"}
	if r.Name() != "fenced-code-style" {
		t.Errorf("expected name fenced-code-style, got %s", r.Name())
	}
}
