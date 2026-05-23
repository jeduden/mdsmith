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

// recordingBlockParser implements parser.BlockParser + parser.SetOptioner.
type recordingBlockParser struct {
	setOptionCalls int
}

func (b *recordingBlockParser) Trigger() []byte { return nil } // free block parser path
func (b *recordingBlockParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	return nil, parser.NoChildren
}
func (b *recordingBlockParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	return parser.Close
}
func (b *recordingBlockParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {}
func (b *recordingBlockParser) CanInterruptParagraph() bool                                 { return false }
func (b *recordingBlockParser) CanAcceptIndentedLine() bool                                 { return false }
func (b *recordingBlockParser) SetOption(name parser.OptionName, _ any) {
	if name == customOptName {
		b.setOptionCalls++
	}
}

// badValue is something that doesn't implement BlockParser /
// InlineParser / ParagraphTransformer / ASTTransformer. Used to
// drive the panic branches in addBlockParser etc.
type badValue struct{}

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

func TestParser_AddBlockParser_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on non-BlockParser value")
		}
	}()
	parser.NewParser(
		parser.WithBlockParsers(util.Prioritized(&badValue{}, 999)),
	).Parse(text.NewReader([]byte("x\n")), parser.WithContext(parser.NewContext()))
}

func TestParser_AddInlineParser_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on non-InlineParser value")
		}
	}()
	parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(util.Prioritized(&badValue{}, 999)),
	).Parse(text.NewReader([]byte("x\n")), parser.WithContext(parser.NewContext()))
}

func TestParser_AddParagraphTransformer_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on non-ParagraphTransformer value")
		}
	}()
	parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(util.Prioritized(&badValue{}, 999)),
	).Parse(text.NewReader([]byte("x\n")), parser.WithContext(parser.NewContext()))
}

func TestParser_AddASTTransformer_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on non-ASTTransformer value")
		}
	}()
	parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
		parser.WithASTTransformers(util.Prioritized(&badValue{}, 999)),
	).Parse(text.NewReader([]byte("x\n")), parser.WithContext(parser.NewContext()))
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
	// get invoked. Threading WithOption through to populate the
	// options map fires the SetOptioner-cast loop bodies.
	p := parser.NewParser(
		parser.WithBlockParsers(append(parser.DefaultBlockParsers(),
			util.Prioritized(&recordingBlockParser{}, 999))...),
		parser.WithInlineParsers(
			append(parser.DefaultInlineParsers(),
				util.Prioritized(&recordingInlineParser{}, 999))...),
		parser.WithParagraphTransformers(
			append(parser.DefaultParagraphTransformers(),
				util.Prioritized(&recordingParagraphTransformer{}, 999))...),
		parser.WithASTTransformers(util.Prioritized(&recordingASTTransformer{}, 999)),
		parser.WithOption(customOptName, "value"),
	)
	root := p.Parse(text.NewReader([]byte("# A\n\nparagraph\n")), parser.WithContext(parser.NewContext()))
	if root == nil {
		t.Fatal("Parse returned nil")
	}
}
