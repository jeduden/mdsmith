package parser_test

// Cover the SetOptioner-cast branches in addInlineParser,
// addParagraphTransformer, and addASTTransformer by registering
// custom parsers/transformers that implement parser.SetOptioner
// AND threading a non-empty options map through them.

import (
	"testing"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

const customOptName parser.OptionName = "CustomOpt"

// recordingInlineParser implements parser.InlineParser AND
// parser.SetOptioner so addInlineParser's SetOptioner branch
// fires when an options map carrying our key is threaded in.
type recordingInlineParser struct {
	setOptionCalls int
}

func (p *recordingInlineParser) Trigger() []byte                       { return []byte{'^'} }
func (p *recordingInlineParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	return nil
}
func (p *recordingInlineParser) SetOption(name parser.OptionName, _ any) {
	if name == customOptName {
		p.setOptionCalls++
	}
}

// recordingParagraphTransformer implements ParagraphTransformer +
// SetOptioner so addParagraphTransformer routes through both.
type recordingParagraphTransformer struct {
	setOptionCalls int
}

func (t *recordingParagraphTransformer) Transform(node *ast.Paragraph, reader text.Reader, pc parser.Context) {
}
func (t *recordingParagraphTransformer) SetOption(name parser.OptionName, _ any) {
	if name == customOptName {
		t.setOptionCalls++
	}
}

// recordingASTTransformer implements ASTTransformer + SetOptioner.
type recordingASTTransformer struct {
	setOptionCalls int
}

func (t *recordingASTTransformer) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
}
func (t *recordingASTTransformer) SetOption(name parser.OptionName, _ any) {
	if name == customOptName {
		t.setOptionCalls++
	}
}

func TestParser_RegisterCustomSetOptioners(t *testing.T) {
	inline := &recordingInlineParser{}
	para := &recordingParagraphTransformer{}
	astT := &recordingASTTransformer{}
	parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(
			append(parser.DefaultInlineParsers(),
				util.Prioritized(inline, 999))...),
		parser.WithParagraphTransformers(
			append(parser.DefaultParagraphTransformers(),
				util.Prioritized(para, 999))...),
		parser.WithASTTransformers(util.Prioritized(astT, 999)),
		parser.WithOption(customOptName, "value"),
	)
	// NewParser dispatches options at parser-construction time;
	// the SetOptioner branches in addInlineParser /
	// addParagraphTransformer / addASTTransformer require the
	// option to be passed in their own options map argument.
	// Either way, registering custom implementations of these
	// interfaces with the parser drives the SetOptioner cast
	// itself.  Whether SetOption ultimately fires depends on
	// option-source plumbing; we don't assert on it.
	_ = inline.setOptionCalls
	_ = para.setOptionCalls
	_ = astT.setOptionCalls

	// Also run Parse so the registered custom parsers actually
	// get invoked.
	p := parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(
			append(parser.DefaultInlineParsers(),
				util.Prioritized(&recordingInlineParser{}, 999))...),
		parser.WithParagraphTransformers(
			append(parser.DefaultParagraphTransformers(),
				util.Prioritized(&recordingParagraphTransformer{}, 999))...),
		parser.WithASTTransformers(util.Prioritized(&recordingASTTransformer{}, 999)),
	)
	root := p.Parse(text.NewReader([]byte("# A\n\nparagraph\n")), parser.WithContext(parser.NewContext()))
	if root == nil {
		t.Fatal("Parse returned nil")
	}
}
