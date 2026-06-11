//go:build goldmark_upstream

package parser_test

import (
	"testing"

	"github.com/jeduden/mdsmith/pkg/goldmark/arena"
	"github.com/jeduden/mdsmith/pkg/goldmark/parser"
	"github.com/jeduden/mdsmith/pkg/goldmark/text"
)

// TestParseIgnoresCallerArenaUpstream pins the WithArena contract on
// the goldmark_upstream axis: newArenaForParse returns nil there, and
// a caller-supplied arena must be ignored so the harness keeps
// exercising the true upstream allocation path (see ParseConfig.Arena).
func TestParseIgnoresCallerArenaUpstream(t *testing.T) {
	p := parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
	)
	src := []byte("# Heading\n\nA paragraph with *emphasis* and `code`.\n")

	a := arena.New()
	node := p.Parse(text.NewReader(src), parser.WithArena(a))
	if node == nil {
		t.Fatal("Parse returned nil node")
	}
	if got := a.TextsAllocated(); got != 0 {
		t.Fatalf("caller arena gained %d texts; the upstream axis must ignore WithArena", got)
	}
}
