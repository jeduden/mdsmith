package ext

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// SubscriptNode is the AST node produced by the subscript parser for
// a single-tilde `~text~` span.
type SubscriptNode struct {
	ast.BaseInline
}

// KindSubscript is the NodeKind of SubscriptNode.
var KindSubscript = ast.NewNodeKind("Subscript")

// Kind implements ast.Node.
func (n *SubscriptNode) Kind() ast.NodeKind { return KindSubscript }

// Dump implements ast.Node for debug output.
func (n *SubscriptNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

// subscriptDelimiter drives the delimiter framework over a single-
// tilde span. Goldmark's built-in strikethrough uses the same char
// but matches `~~…~~`; see subscriptParser.Parse for the length
// partition that keeps the two extensions from stepping on each
// other.
type subscriptDelimiter struct{}

func (p *subscriptDelimiter) IsDelimiter(b byte) bool { return b == '~' }

func (p *subscriptDelimiter) CanOpenCloser(opener, closer *parser.Delimiter) bool {
	return opener.Char == '~' && closer.Char == '~'
}

func (p *subscriptDelimiter) OnMatch(consumes int) ast.Node { return &SubscriptNode{} }

var defaultSubscriptDelimiter = &subscriptDelimiter{}

// subscriptParser is the InlineParser registered with goldmark.
type subscriptParser struct{}

// Trigger implements parser.InlineParser.
func (p *subscriptParser) Trigger() []byte { return []byte{'~'} }

// Parse implements parser.InlineParser.
//
// Subscript shares its trigger byte with goldmark's strikethrough
// extension. The two coexist through two complementary rules:
//
//   - Subscript accepts only an exactly length-1 `~` run (`~text~`).
//     `~~...~~` is rejected here so strikethrough — registered at
//     a lower priority — gets to handle it next.
//   - A Parse call that starts immediately after another `~` is
//     rejected, so goldmark advancing one byte into the middle of
//     `~~` cannot trigger a spurious subscript span.
func (p *subscriptParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	before := block.PrecendingCharacter()
	if before == '~' {
		return nil
	}
	line, segment := block.PeekLine()
	node := parser.ScanDelimiter(line, before, 1, defaultSubscriptDelimiter)
	if node == nil || node.OriginalLength != 1 {
		return nil
	}
	node.Segment = segment.WithStop(segment.Start + node.OriginalLength)
	block.Advance(node.OriginalLength)
	pc.PushDelimiter(node)
	return node
}

// CloseBlock implements parser.InlineParser.
func (p *subscriptParser) CloseBlock(parent ast.Node, pc parser.Context) {}

// subscriptExt wires the parser into goldmark.
type subscriptExt struct{}

// Subscript is the goldmark Extender that installs the subscript
// inline parser with a priority (400) higher — i.e. numerically
// smaller — than built-in strikethrough (500). Goldmark tries inline
// parsers for a shared trigger byte in priority order and stops at
// the first non-nil result, so subscript takes length-1 runs and
// strikethrough still handles length-2 runs.
var Subscript goldmark.Extender = &subscriptExt{}

// Extend implements goldmark.Extender.
func (e *subscriptExt) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithInlineParsers(
		util.Prioritized(&subscriptParser{}, 400),
	))
}
