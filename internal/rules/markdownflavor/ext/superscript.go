// Package ext implements detection-only goldmark extensions used by
// MDS034 (markdown-flavor) to flag syntax that varies across Markdown
// flavors. Each extension parses its feature's syntax into a custom
// AST node; the rule walks the dual parser's tree and emits
// diagnostics. There is no HTML renderer — the nodes exist purely so
// the main rule can detect them by kind.
package ext

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// SuperscriptNode is the AST node produced by the superscript parser
// for a `^text^` span. It carries no extra state; the surrounding
// content is stored as inline children.
type SuperscriptNode struct {
	ast.BaseInline
}

// KindSuperscript is the NodeKind of SuperscriptNode.
var KindSuperscript = ast.NewNodeKind("Superscript")

// Kind implements ast.Node.
func (n *SuperscriptNode) Kind() ast.NodeKind { return KindSuperscript }

// Dump implements ast.Node for debug output.
func (n *SuperscriptNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

// superscriptDelimiter drives goldmark's delimiter pairing over `^`.
// A `^` run of length 1 can open or close a superscript span; longer
// runs (e.g. `^^`) are rejected so they remain literal text.
type superscriptDelimiter struct{}

func (p *superscriptDelimiter) IsDelimiter(b byte) bool { return b == '^' }

func (p *superscriptDelimiter) CanOpenCloser(opener, closer *parser.Delimiter) bool {
	return opener.Char == '^' && closer.Char == '^'
}

func (p *superscriptDelimiter) OnMatch(consumes int) ast.Node {
	return &SuperscriptNode{}
}

var defaultSuperscriptDelimiter = &superscriptDelimiter{}

// superscriptParser is the InlineParser registered with goldmark.
type superscriptParser struct{}

// Trigger implements parser.InlineParser.
func (p *superscriptParser) Trigger() []byte { return []byte{'^'} }

// Parse implements parser.InlineParser. It rejects `^^` and longer
// runs so they remain literal, and pushes a length-1 delimiter that
// goldmark's delimiter framework pairs with the next `^` in the same
// inline context. Spans containing whitespace (`^ x ^`) are rejected
// via CanOpen/CanClose, matching the emphasis-style left/right-flank
// rules that parser.ScanDelimiter computes.
func (p *superscriptParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	before := block.PrecendingCharacter()
	// `^^` / `^^^` and longer runs must stay literal. When goldmark
	// advances into the middle of such a run, the `before` rune is
	// `^` — reject those positions too so a stray single `^` inside
	// a longer run does not start a bogus span.
	if before == '^' {
		return nil
	}
	line, segment := block.PeekLine()
	node := parser.ScanDelimiter(line, before, 1, defaultSuperscriptDelimiter)
	if node == nil || node.OriginalLength != 1 {
		return nil
	}
	node.Segment = segment.WithStop(segment.Start + node.OriginalLength)
	block.Advance(node.OriginalLength)
	pc.PushDelimiter(node)
	return node
}

// CloseBlock implements parser.InlineParser.
func (p *superscriptParser) CloseBlock(parent ast.Node, pc parser.Context) {}

// superscriptExt wires the parser into goldmark. It registers only
// the parser; MDS034 does not render, so no HTML renderer is added.
type superscriptExt struct{}

// Superscript is the goldmark Extender that installs the superscript
// inline parser at priority 500. In goldmark a lower priority number
// runs earlier, so CommonMark emphasis (100) still wins on its own
// delimiters; `^` has no other default parser, so the ordering here
// does not introduce ambiguity.
var Superscript goldmark.Extender = &superscriptExt{}

// Extend implements goldmark.Extender.
func (e *superscriptExt) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithInlineParsers(
		util.Prioritized(&superscriptParser{}, 500),
	))
}
