package markdown_test

import (
	"testing"

	"github.com/jeduden/mdsmith/pkg/goldmark/arena"
	"github.com/jeduden/mdsmith/pkg/goldmark/parser"
	"github.com/jeduden/mdsmith/pkg/markdown"
)

// TestParseContextArena pins the caller-arena entry point: the parse
// records link references in the supplied context exactly like
// ParseContext, and the AST's Text nodes come from the caller arena.
func TestParseContextArena(t *testing.T) {
	src := []byte("# T\n\nSee [ref][r].\n\n[r]: https://example.com\n")
	a := arena.New()
	ctx := parser.NewContext()
	node := markdown.ParseContextArena(src, ctx, a)
	if node == nil {
		t.Fatal("nil AST")
	}
	if a.TextsAllocated() == 0 {
		t.Fatal("caller arena unused")
	}
	if len(ctx.References()) != 1 {
		t.Fatalf("references = %d, want 1", len(ctx.References()))
	}
}
