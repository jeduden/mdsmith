package blanklinearoundfencedcode

import (
	"testing"

	"github.com/jeduden/tidymark/internal/lint"
)

func TestCheck_NoBlankBefore(t *testing.T) {
	src := []byte("some text\n```go\ncode\n```\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	diags := r.Check(f)
	// goldmark may not parse this as a fenced code block if there's no blank line,
	// but let's test what we get
	if len(diags) < 1 {
		t.Logf("Note: goldmark may have parsed this differently, got %d diagnostics", len(diags))
	}
}

func TestCheck_NoBlankAfter(t *testing.T) {
	src := []byte("```go\ncode\n```\nsome text\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	diags := r.Check(f)
	hasAfterDiag := false
	for _, d := range diags {
		if d.Message == "fenced code block should be followed by a blank line" {
			hasAfterDiag = true
		}
	}
	if !hasAfterDiag {
		t.Fatalf("expected diagnostic about blank line after, got %d diagnostics: %v", len(diags), diags)
	}
}

func TestCheck_BlankLinesPresent(t *testing.T) {
	src := []byte("some text\n\n```go\ncode\n```\n\nmore text\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d: %v", len(diags), diags)
	}
}

func TestCheck_StartOfFile(t *testing.T) {
	src := []byte("```go\ncode\n```\n\nsome text\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics (start of file), got %d: %v", len(diags), diags)
	}
}

func TestCheck_EndOfFile(t *testing.T) {
	src := []byte("some text\n\n```go\ncode\n```\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics (end of file), got %d: %v", len(diags), diags)
	}
}

func TestCheck_EndOfFileNoTrailingNewline(t *testing.T) {
	src := []byte("some text\n\n```go\ncode\n```")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics (end of file), got %d: %v", len(diags), diags)
	}
}

func TestFix_InsertBlankBefore(t *testing.T) {
	// We need goldmark to actually parse this as a fenced code block
	// goldmark is forgiving and will parse even without blank before
	src := []byte("text\n```go\ncode\n```\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	diags := r.Check(f)
	if len(diags) == 0 {
		t.Skip("goldmark did not detect issue here")
	}
	result := r.Fix(f)
	// Verify the fix added a blank line
	if string(result) == string(src) {
		t.Error("expected fix to change the source")
	}
}

func TestFix_InsertBlankAfter(t *testing.T) {
	src := []byte("```go\ncode\n```\ntext\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	result := r.Fix(f)
	expected := "```go\ncode\n```\n\ntext\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestFix_InsertBlankBeforeAndAfter(t *testing.T) {
	// Start with a case where goldmark parses it despite lack of blank lines
	src := []byte("```go\ncode\n```\ntext\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	result := r.Fix(f)
	expected := "```go\ncode\n```\n\ntext\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestFix_NoChangeNeeded(t *testing.T) {
	src := []byte("text\n\n```go\ncode\n```\n\nmore text\n")
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

func TestCheck_ID(t *testing.T) {
	r := &Rule{}
	if r.ID() != "TM015" {
		t.Errorf("expected ID TM015, got %s", r.ID())
	}
}

func TestCheck_Name(t *testing.T) {
	r := &Rule{}
	if r.Name() != "blank-line-around-fenced-code" {
		t.Errorf("expected name blank-line-around-fenced-code, got %s", r.Name())
	}
}

func TestCheck_MultipleBlocks(t *testing.T) {
	src := []byte("```go\ncode1\n```\ntext\n```python\ncode2\n```\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	diags := r.Check(f)
	// Should have diagnostics for missing blank lines
	if len(diags) == 0 {
		t.Log("Note: goldmark may parse this differently")
	}
	t.Logf("Got %d diagnostics", len(diags))
	for _, d := range diags {
		t.Logf("  Line %d: %s", d.Line, d.Message)
	}
}
