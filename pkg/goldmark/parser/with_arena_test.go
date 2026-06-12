//go:build !goldmark_upstream

package parser_test

import (
	"testing"

	"github.com/jeduden/mdsmith/pkg/goldmark/arena"
	"github.com/jeduden/mdsmith/pkg/goldmark/parser"
	"github.com/jeduden/mdsmith/pkg/goldmark/text"
)

// TestParseWithCallerArena pins the WithArena contract: when the
// caller supplies an arena, the parse draws its Text nodes from it
// (observable through TextsAllocated), and a nil option leaves the
// per-parse default in place.
func TestParseWithCallerArena(t *testing.T) {
	p := parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
	)
	src := []byte("# Heading\n\nA paragraph with *emphasis* and `code`.\n")

	a := arena.New()
	if got := a.TextsAllocated(); got != 0 {
		t.Fatalf("fresh arena reports %d texts", got)
	}
	node := p.Parse(text.NewReader(src), parser.WithArena(a))
	if node == nil {
		t.Fatal("Parse returned nil node")
	}
	if got := a.TextsAllocated(); got == 0 {
		t.Fatalf("caller arena received no Text allocations; WithArena not honoured")
	}

	// Without the option the parse must not touch the caller arena.
	a2 := arena.New()
	_ = p.Parse(text.NewReader(src))
	if got := a2.TextsAllocated(); got != 0 {
		t.Fatalf("unrelated arena gained %d texts", got)
	}
}
