package parser_test

// Direct-call coverage for the predicate methods on block
// parsers: CanInterruptParagraph and CanAcceptIndentedLine on the
// atx-heading and blockquote parsers, Close on the blockquote
// parser, plus WithAutoHeadingID option dispatching through
// SetHeadingOption / SetParserOption.

import (
	"testing"

	"github.com/yuin/goldmark/parser"
)

func TestATXHeading_DirectPredicates(t *testing.T) {
	p := parser.NewATXHeadingParser()
	if !p.CanInterruptParagraph() {
		t.Error("ATX heading parser should interrupt paragraphs")
	}
	if p.CanAcceptIndentedLine() {
		t.Error("ATX heading parser should not accept indented lines")
	}
}

func TestBlockquote_DirectPredicates(t *testing.T) {
	p := parser.NewBlockquoteParser()
	if !p.CanInterruptParagraph() {
		t.Error("blockquote should interrupt paragraphs")
	}
	if p.CanAcceptIndentedLine() {
		t.Error("blockquote should not accept indented lines")
	}
	// Close is a no-op; just call it to clear the 0% mark.
	p.Close(nil, nil, parser.NewContext())
}

func TestATXHeading_WithAutoHeadingID(t *testing.T) {
	// WithAutoHeadingID returns a typed option whose
	// SetHeadingOption / SetParserOption methods are dispatched
	// only when the option flows through ATXHeadingParser. Drive
	// both paths by using the option on a NewATXHeadingParser
	// directly and through WithParserOptions on a constructed
	// Parser.
	p := parser.NewATXHeadingParser(parser.WithAutoHeadingID())
	if p == nil {
		t.Fatal("constructor returned nil")
	}

	// Through the parser-options dispatcher: WithAutoHeadingID
	// also implements parser.Option (SetParserOption), so it can
	// be threaded through WithParserOptions too.
	parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
		parser.WithAutoHeadingID(),
	)
}

func TestAttributes_FindFromTopLevel(t *testing.T) {
	// The top-level parser.Attributes.Find helper is unreachable
	// via the normal parse-flow because parsers consume attributes
	// through ParseAttributes directly. Construct one and call
	// Find to clear the 0 % mark.
	attrs := parser.Attributes{
		parser.Attribute{Name: []byte("id"), Value: []byte("a")},
		parser.Attribute{Name: []byte("class"), Value: []byte("c")},
	}
	if _, ok := attrs.Find([]byte("id")); !ok {
		t.Error("Find should locate 'id'")
	}
	if _, ok := attrs.Find([]byte("missing")); ok {
		t.Error("Find should not locate missing keys")
	}
}
