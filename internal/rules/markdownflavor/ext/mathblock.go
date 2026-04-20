package ext

import (
	"bytes"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// MathBlockNode is the AST node produced by the math-block parser
// for a `$$...$$` fenced display-math block. Detection-only; body
// bytes are recorded via BaseBlock.Lines so a future Fix could slice
// them out of the source, but MDS034 only inspects the node's kind.
type MathBlockNode struct {
	ast.BaseBlock
	// closed tracks whether a closing `$$` fence was observed.
	closed bool
}

// KindMathBlock is the NodeKind of MathBlockNode.
var KindMathBlock = ast.NewNodeKind("MathBlock")

// Kind implements ast.Node.
func (n *MathBlockNode) Kind() ast.NodeKind { return KindMathBlock }

// IsRaw implements ast.Node. Math-block content is raw — no inline
// children are parsed inside.
func (n *MathBlockNode) IsRaw() bool { return true }

// HasClosure reports whether the block observed a closing fence.
func (n *MathBlockNode) HasClosure() bool { return n.closed }

// Dump implements ast.Node for debug output.
func (n *MathBlockNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

// mathBlockParser is the BlockParser registered with goldmark.
type mathBlockParser struct{}

// Trigger implements parser.BlockParser.
func (p *mathBlockParser) Trigger() []byte { return []byte{'$'} }

var mathFence = []byte("$$")

// Open implements parser.BlockParser. A line that starts with `$$`
// (after up to three spaces of indent) at the document root opens a
// math-block node. If the same line also contains a closing `$$`,
// the block is closed immediately.
func (p *mathBlockParser) Open(
	parent ast.Node, reader text.Reader, pc parser.Context,
) (ast.Node, parser.State) {
	line, seg := reader.PeekLine()
	if line == nil {
		return nil, parser.NoChildren
	}
	trimmed := bytes.TrimLeft(line, " ")
	indent := len(line) - len(trimmed)
	if indent >= 4 {
		return nil, parser.NoChildren
	}
	if !bytes.HasPrefix(trimmed, mathFence) {
		return nil, parser.NoChildren
	}
	node := &MathBlockNode{}
	node.Lines().Append(seg)

	// Look for a closing `$$` on the same line. The rest of the line
	// (after the opening `$$`) is searched for a standalone `$$`.
	rest := bytes.TrimRight(trimmed[len(mathFence):], "\r\n")
	if bytes.Contains(rest, mathFence) {
		node.closed = true
	}
	reader.AdvanceToEOL()
	return node, parser.NoChildren
}

// Continue implements parser.BlockParser. Each subsequent line is
// appended to the block. A line whose trimmed content is exactly
// `$$` closes the block.
func (p *mathBlockParser) Continue(
	n ast.Node, reader text.Reader, pc parser.Context,
) parser.State {
	mb := n.(*MathBlockNode)
	if mb.closed {
		return parser.Close
	}
	line, seg := reader.PeekLine()
	if line == nil {
		return parser.Close
	}
	mb.Lines().Append(seg)
	if bytes.Equal(bytes.TrimSpace(line), mathFence) {
		mb.closed = true
		reader.AdvanceToEOL()
		return parser.Close
	}
	reader.AdvanceToEOL()
	return parser.Continue | parser.NoChildren
}

// Close implements parser.BlockParser.
func (p *mathBlockParser) Close(n ast.Node, reader text.Reader, pc parser.Context) {}

// CanInterruptParagraph implements parser.BlockParser. Math fences
// behave like fenced code — they can start a block on a new line
// but do not interrupt an existing paragraph.
func (p *mathBlockParser) CanInterruptParagraph() bool { return false }

// CanAcceptIndentedLine implements parser.BlockParser.
func (p *mathBlockParser) CanAcceptIndentedLine() bool { return false }

// mathBlockExt wires the parser into goldmark.
type mathBlockExt struct{}

// MathBlock is the goldmark Extender that installs the math-block
// block parser at priority 700 — lower than goldmark's fenced-code
// parser (700) — so fenced-code still wins on backtick runs.
var MathBlock goldmark.Extender = &mathBlockExt{}

// Extend implements goldmark.Extender.
func (e *mathBlockExt) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithBlockParsers(
		util.Prioritized(&mathBlockParser{}, 700),
	))
}
