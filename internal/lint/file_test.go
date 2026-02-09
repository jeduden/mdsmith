package lint

import (
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
