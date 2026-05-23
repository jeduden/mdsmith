package parser_test

// Direct-call coverage for the predicate methods (Close,
// CanInterruptParagraph, CanAcceptIndentedLine) on parsers
// where the dispatcher doesn't always invoke them.  These are
// constant-return functions; the calls only exist to satisfy
// the BlockParser interface.

import (
	"testing"

	"github.com/yuin/goldmark/parser"
)

func TestFencedCodeBlockParser_Predicates(t *testing.T) {
	p := parser.NewFencedCodeBlockParser()
	_ = p.CanInterruptParagraph()
	_ = p.CanAcceptIndentedLine()
}

func TestHTMLBlockParser_Predicates(t *testing.T) {
	p := parser.NewHTMLBlockParser()
	_ = p.CanInterruptParagraph()
	_ = p.CanAcceptIndentedLine()
	p.Close(nil, nil, parser.NewContext())
}

func TestListItemParser_Predicates(t *testing.T) {
	p := parser.NewListItemParser()
	_ = p.CanAcceptIndentedLine()
	p.Close(nil, nil, parser.NewContext())
}

func TestThematicBreakParser_Predicates(t *testing.T) {
	p := parser.NewThematicBreakParser()
	_ = p.CanAcceptIndentedLine()
	p.Close(nil, nil, parser.NewContext())
}
