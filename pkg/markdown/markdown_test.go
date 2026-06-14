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

// TestParseContextArenaStructuralNodes pins that a real parse draws its
// Heading and ListItem nodes from the caller arena, not the heap — the
// wiring that keeps the heading- and list-heavy neutral benchmark corpus
// off the per-file allocation path.
func TestParseContextArenaStructuralNodes(t *testing.T) {
	src := []byte("# H1\n\nSetext\n======\n\n- one\n- two\n- three\n")
	a := arena.New()
	ctx := parser.NewContext()
	if node := markdown.ParseContextArena(src, ctx, a); node == nil {
		t.Fatal("nil AST")
	}
	// Two headings (ATX "# H1" + setext "Setext\n===") and three list
	// items must all come from the arena.
	if got := a.HeadingsAllocated(); got < 2 {
		t.Fatalf("HeadingsAllocated = %d, want >= 2 (arena unused for headings)", got)
	}
	if got := a.ListItemsAllocated(); got < 3 {
		t.Fatalf("ListItemsAllocated = %d, want >= 3 (arena unused for list items)", got)
	}
}
