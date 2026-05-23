package parser_test

// Verify that paragraphs with MULTIPLE link-reference definitions
// at the head (or interleaved with text) produce the expected
// link-ref nodes and leave the right tail of the paragraph behind.
// This drives the `offset := 0; for _, remove := range removes`
// compaction logic in link_ref.go through more than one iteration.

import (
	"testing"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

func parseDefault(src string) ast.Node {
	p := parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
	)
	return p.Parse(text.NewReader([]byte(src)), parser.WithContext(parser.NewContext()))
}

func countKind(root ast.Node, kind ast.NodeKind) int {
	n := 0
	_ = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && node.Kind() == kind {
			n++
		}
		return ast.WalkContinue, nil
	})
	return n
}

func TestLinkRef_MultipleConsecutiveDefs(t *testing.T) {
	src := `[a]: /a
[b]: /b
[c]: /c

use [a], [b], [c]
`
	root := parseDefault(src)
	if got := countKind(root, ast.KindLinkReferenceDefinition); got != 3 {
		t.Errorf("got %d LinkReferenceDefinition nodes, want 3", got)
	}
	if got := countKind(root, ast.KindLink); got != 3 {
		t.Errorf("got %d Link nodes, want 3", got)
	}
}

func TestLinkRef_TwoDefsThenText(t *testing.T) {
	src := `[a]: /a
[b]: /b
trailing text

use [a] and [b]
`
	root := parseDefault(src)
	if got := countKind(root, ast.KindLinkReferenceDefinition); got != 2 {
		t.Errorf("got %d LinkReferenceDefinition nodes, want 2", got)
	}
	// The trailing text must survive as a Paragraph child.
	if got := countKind(root, ast.KindParagraph); got < 1 {
		t.Errorf("got %d Paragraph nodes, want >= 1", got)
	}
}

func TestLinkRef_TextThenTwoDefs(t *testing.T) {
	// CommonMark link-ref defs must appear at the start of a
	// paragraph (no text before). Leading text means none of these
	// are treated as defs.
	src := `intro line
[a]: /a
[b]: /b

use [a] and [b]
`
	root := parseDefault(src)
	if got := countKind(root, ast.KindLinkReferenceDefinition); got != 0 {
		t.Errorf("with leading text got %d LinkReferenceDefinition nodes, want 0", got)
	}
}

func TestLinkRef_ThreeDefsInterleavedSpacing(t *testing.T) {
	src := `[a]: /a
   [b]: /b "title"
[c]: /c (title)

[a] [b] [c]
`
	root := parseDefault(src)
	if got := countKind(root, ast.KindLinkReferenceDefinition); got != 3 {
		t.Errorf("got %d LinkReferenceDefinition nodes, want 3", got)
	}
}
