package ext

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// MathInlineNode is the AST node produced by the inline-math parser
// for a `$...$` span using Pandoc's tex_math_dollars rules.
type MathInlineNode struct {
	ast.BaseInline
}

// KindMathInline is the NodeKind of MathInlineNode.
var KindMathInline = ast.NewNodeKind("MathInline")

// Kind implements ast.Node.
func (n *MathInlineNode) Kind() ast.NodeKind { return KindMathInline }

// Dump implements ast.Node for debug output.
func (n *MathInlineNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

// mathInlineParser is the InlineParser registered with goldmark.
// It walks the current line byte-by-byte to apply Pandoc's
// tex_math_dollars rules, which are not expressible through the
// delimiter-pairing framework because the closing `$` depends on
// the character that *follows* it.
type mathInlineParser struct{}

// Trigger implements parser.InlineParser.
func (p *mathInlineParser) Trigger() []byte { return []byte{'$'} }

// Parse implements parser.InlineParser.
//
// Pandoc's tex_math_dollars:
//   - The opening `$` must be immediately followed by a character
//     that is not whitespace and not another `$` (the latter rule
//     keeps `$$` from looking like a zero-length inline span).
//   - A closing `$` is any `$` on the same line whose preceding
//     character is not whitespace and whose following character is
//     not a digit.
//
// This matches `$x$`, `($x$)`, and `foo $x+1$ bar` while rejecting
// `$ x $`, `$x $`, `$20`, and `$$`.
func (p *mathInlineParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	before := block.PrecendingCharacter()
	if before == '$' {
		return nil
	}
	line, segment := block.PeekLine()
	if len(line) < 2 || line[0] != '$' {
		return nil
	}
	next := line[1]
	if next == '$' || isSpaceByte(next) {
		return nil
	}
	// Find a closing `$` on the line.
	closeIdx := -1
	for i := 2; i < len(line); i++ {
		if line[i] != '$' {
			continue
		}
		prev := line[i-1]
		if isSpaceByte(prev) {
			continue
		}
		// If followed by another `$`, this is a `$$` fence marker,
		// not a valid math-inline closer.
		if i+1 < len(line) && line[i+1] == '$' {
			continue
		}
		if i+1 < len(line) && isDigitByte(line[i+1]) {
			continue
		}
		closeIdx = i
		break
	}
	if closeIdx < 0 {
		return nil
	}
	node := &MathInlineNode{}
	contentSeg := segment.WithStart(segment.Start + 1)
	contentSeg = contentSeg.WithStop(segment.Start + closeIdx)
	node.AppendChild(node, ast.NewTextSegment(contentSeg))
	block.Advance(closeIdx + 1)
	return node
}

// CloseBlock implements parser.InlineParser.
func (p *mathInlineParser) CloseBlock(parent ast.Node, pc parser.Context) {}

// isSpaceByte reports whether b is an ASCII whitespace byte the
// Pandoc rule treats as "space" (space, tab, newline, CR).
func isSpaceByte(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// isDigitByte reports whether b is an ASCII decimal digit.
func isDigitByte(b byte) bool { return b >= '0' && b <= '9' }

// mathInlineExt wires the parser into goldmark.
type mathInlineExt struct{}

// MathInline is the goldmark Extender that installs the math-inline
// parser at priority 200 — higher than emphasis (100 is reserved;
// CommonMark emphasis registers at 100) — well, actually a lower
// number means higher priority in goldmark. Use 200 so Pandoc-style
// math beats plain-text handling but does not interfere with any
// other `$`-using parser (there is none by default).
var MathInline goldmark.Extender = &mathInlineExt{}

// Extend implements goldmark.Extender.
func (e *mathInlineExt) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithInlineParsers(
		util.Prioritized(&mathInlineParser{}, 200),
	))
}
