//go:build !goldmark_upstream

package parser_test

import (
	"testing"

	"github.com/jeduden/mdsmith/pkg/goldmark/arena"
	"github.com/jeduden/mdsmith/pkg/goldmark/parser"
	"github.com/jeduden/mdsmith/pkg/goldmark/text"
)

// TestParseWithCallerArena_EmphasisDrawsFromArena pins the fix for
// docs/development/high-performance-go.md's arena-allocation
// guidance: emphasisDelimiterProcessor.OnMatch called ast.NewEmphasis
// directly, a plain heap allocation, even though arena.Arena already
// exposes a pooled Emphasis method every other frequently-built node
// type (Paragraph, Heading, ListItem, CodeSpan, Link, RawHTML, Text)
// routes through via ArenaForContext. Emphasis/strong markup is one
// of the most common inline constructs in real Markdown prose, so an
// unrouted OnMatch is a heap allocation on the single canonical parse
// every rule in mdsmith consults, for every *em*/_em_/**strong** span
// in every file.
func TestParseWithCallerArena_EmphasisDrawsFromArena(t *testing.T) {
	p := parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
	)
	src := []byte("A paragraph with *emphasis* and **strong** text.\n")

	a := arena.New()
	if got := a.EmphasesAllocated(); got != 0 {
		t.Fatalf("fresh arena reports %d emphases", got)
	}
	node := p.Parse(text.NewReader(src), parser.WithArena(a))
	if node == nil {
		t.Fatal("Parse returned nil node")
	}
	// Two emphasis-family spans in src: one *emphasis* (level 1), one
	// **strong** (level 2).
	if got := a.EmphasesAllocated(); got != 2 {
		t.Fatalf("caller arena received %d emphasis allocations, want 2; OnMatch not routed through the arena", got)
	}
}
