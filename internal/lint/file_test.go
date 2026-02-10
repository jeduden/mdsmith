package lint

import (
	"bytes"
	"testing"

	"github.com/yuin/goldmark/ast"
)

func TestNewFile_EmptyContent(t *testing.T) {
	f, err := NewFile("test.md", []byte(""))
	if err != nil {
		t.Fatalf("NewFile returned error: %v", err)
	}
	if f.AST == nil {
		t.Fatal("expected non-nil AST for empty content")
	}
	if f.AST.Kind() != ast.KindDocument {
		t.Errorf("expected Document node, got %v", f.AST.Kind())
	}
	if f.Path != "test.md" {
		t.Errorf("expected path %q, got %q", "test.md", f.Path)
	}
}

func TestNewFile_WithMarkdownContent(t *testing.T) {
	source := []byte("# Heading\n\nSome text.\n\n- item 1\n- item 2\n\n```go\nfmt.Println()\n```\n")
	f, err := NewFile("doc.md", source)
	if err != nil {
		t.Fatalf("NewFile returned error: %v", err)
	}
	if f.AST == nil {
		t.Fatal("expected non-nil AST")
	}
	if f.AST.Kind() != ast.KindDocument {
		t.Errorf("expected Document node, got %v", f.AST.Kind())
	}
	// The document should have child nodes for heading, paragraph, list, code block.
	if !f.AST.HasChildren() {
		t.Error("expected AST to have children for non-empty markdown")
	}
}

func TestNewFile_LinesSplitCorrectly(t *testing.T) {
	source := []byte("line one\nline two\nline three")
	f, err := NewFile("lines.md", source)
	if err != nil {
		t.Fatalf("NewFile returned error: %v", err)
	}
	if len(f.Lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(f.Lines))
	}
	if string(f.Lines[0]) != "line one" {
		t.Errorf("expected first line %q, got %q", "line one", string(f.Lines[0]))
	}
	if string(f.Lines[1]) != "line two" {
		t.Errorf("expected second line %q, got %q", "line two", string(f.Lines[1]))
	}
	if string(f.Lines[2]) != "line three" {
		t.Errorf("expected third line %q, got %q", "line three", string(f.Lines[2]))
	}
}

func TestNewFile_TrailingNewline(t *testing.T) {
	source := []byte("line one\nline two\n")
	f, err := NewFile("trailing.md", source)
	if err != nil {
		t.Fatalf("NewFile returned error: %v", err)
	}
	// bytes.Split on trailing newline produces an empty last element.
	if len(f.Lines) != 3 {
		t.Fatalf("expected 3 lines (including empty trailing), got %d", len(f.Lines))
	}
	if string(f.Lines[2]) != "" {
		t.Errorf("expected empty trailing line, got %q", string(f.Lines[2]))
	}
}

func TestNewFileFromSource_WithFrontMatter(t *testing.T) {
	source := []byte("---\ntitle: hello\n---\n# Heading\n")
	f, err := NewFileFromSource("test.md", source, true)
	if err != nil {
		t.Fatalf("NewFileFromSource returned error: %v", err)
	}

	// FrontMatter should contain the entire prefix including delimiters.
	expectedFM := "---\ntitle: hello\n---\n"
	if string(f.FrontMatter) != expectedFM {
		t.Errorf("FrontMatter = %q, want %q", f.FrontMatter, expectedFM)
	}

	// LineOffset should be 3 (three newlines in the front matter).
	if f.LineOffset != 3 {
		t.Errorf("LineOffset = %d, want 3", f.LineOffset)
	}

	// Source should be the stripped content only.
	expectedSource := "# Heading\n"
	if string(f.Source) != expectedSource {
		t.Errorf("Source = %q, want %q", f.Source, expectedSource)
	}
}

func TestNewFileFromSource_WithoutFrontMatter(t *testing.T) {
	source := []byte("# Heading\nSome text.\n")
	f, err := NewFileFromSource("test.md", source, true)
	if err != nil {
		t.Fatalf("NewFileFromSource returned error: %v", err)
	}

	if f.FrontMatter != nil {
		t.Errorf("FrontMatter = %q, want nil", f.FrontMatter)
	}
	if f.LineOffset != 0 {
		t.Errorf("LineOffset = %d, want 0", f.LineOffset)
	}
	if string(f.Source) != string(source) {
		t.Errorf("Source = %q, want %q", f.Source, source)
	}
}

func TestNewFileFromSource_StripDisabled(t *testing.T) {
	source := []byte("---\ntitle: hello\n---\n# Heading\n")
	f, err := NewFileFromSource("test.md", source, false)
	if err != nil {
		t.Fatalf("NewFileFromSource returned error: %v", err)
	}

	if f.FrontMatter != nil {
		t.Errorf("FrontMatter = %q, want nil", f.FrontMatter)
	}
	if f.LineOffset != 0 {
		t.Errorf("LineOffset = %d, want 0", f.LineOffset)
	}
	// Source should be the full content (front matter not stripped).
	if string(f.Source) != string(source) {
		t.Errorf("Source = %q, want %q", f.Source, source)
	}
}

func TestAdjustDiagnostics_ShiftsLineNumbers(t *testing.T) {
	f := &File{LineOffset: 5}
	diags := []Diagnostic{
		{Line: 1, Column: 3, RuleID: "TM001"},
		{Line: 10, Column: 1, RuleID: "TM002"},
	}
	f.AdjustDiagnostics(diags)

	if diags[0].Line != 6 {
		t.Errorf("diags[0].Line = %d, want 6", diags[0].Line)
	}
	if diags[1].Line != 15 {
		t.Errorf("diags[1].Line = %d, want 15", diags[1].Line)
	}
}

func TestAdjustDiagnostics_ZeroOffsetNoOp(t *testing.T) {
	f := &File{LineOffset: 0}
	diags := []Diagnostic{
		{Line: 1, Column: 1, RuleID: "TM001"},
	}
	f.AdjustDiagnostics(diags)

	if diags[0].Line != 1 {
		t.Errorf("diags[0].Line = %d, want 1", diags[0].Line)
	}
}

func TestFullSource_PrependsFrontMatter(t *testing.T) {
	fm := []byte("---\ntitle: hello\n---\n")
	f := &File{FrontMatter: fm}
	body := []byte("# Heading\n")

	got := f.FullSource(body)
	want := append(fm, body...)
	if !bytes.Equal(got, want) {
		t.Errorf("FullSource = %q, want %q", got, want)
	}
}

func TestFullSource_NoFrontMatter(t *testing.T) {
	f := &File{}
	body := []byte("# Heading\n")

	got := f.FullSource(body)
	if !bytes.Equal(got, body) {
		t.Errorf("FullSource = %q, want %q", got, body)
	}
}

func TestFullSource_DoesNotMutateFrontMatter(t *testing.T) {
	fm := []byte("---\ntitle: hello\n---\n")
	origLen := len(fm)
	origContent := make([]byte, len(fm))
	copy(origContent, fm)

	f := &File{FrontMatter: fm}
	body := []byte("# Heading\n")

	// First call: check result is correct.
	got := f.FullSource(body)
	want := append(origContent, body...)
	if !bytes.Equal(got, want) {
		t.Errorf("FullSource (1st call) = %q, want %q", got, want)
	}

	// Assert FrontMatter is unchanged.
	if len(f.FrontMatter) != origLen {
		t.Errorf("FrontMatter length changed: got %d, want %d", len(f.FrontMatter), origLen)
	}
	if !bytes.Equal(f.FrontMatter, origContent) {
		t.Errorf("FrontMatter content changed: got %q, want %q", f.FrontMatter, origContent)
	}

	// Second call: idempotent result.
	got2 := f.FullSource(body)
	if !bytes.Equal(got2, want) {
		t.Errorf("FullSource (2nd call) = %q, want %q", got2, want)
	}
}

func TestNewFileFromSource_EmptyFrontMatter(t *testing.T) {
	source := []byte("---\n---\n# Heading\n")
	f, err := NewFileFromSource("test.md", source, true)
	if err != nil {
		t.Fatalf("NewFileFromSource returned error: %v", err)
	}

	// LineOffset should be 2 (two newlines in "---\n---\n").
	if f.LineOffset != 2 {
		t.Errorf("LineOffset = %d, want 2", f.LineOffset)
	}

	// FrontMatter should be the delimiters only.
	expectedFM := "---\n---\n"
	if string(f.FrontMatter) != expectedFM {
		t.Errorf("FrontMatter = %q, want %q", f.FrontMatter, expectedFM)
	}

	// Source should be the content after front matter.
	if !bytes.HasPrefix(f.Source, []byte("# Heading")) {
		t.Errorf("Source = %q, want prefix %q", f.Source, "# Heading")
	}
}

func TestNewFileFromSource_EmptySource(t *testing.T) {
	f, err := NewFileFromSource("test.md", []byte(""), true)
	if err != nil {
		t.Fatalf("NewFileFromSource returned error: %v", err)
	}

	if f.LineOffset != 0 {
		t.Errorf("LineOffset = %d, want 0", f.LineOffset)
	}
	if f.FrontMatter != nil {
		t.Errorf("FrontMatter = %q, want nil", f.FrontMatter)
	}
}
