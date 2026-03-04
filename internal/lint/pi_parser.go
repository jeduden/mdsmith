package lint

import (
	"bytes"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

type piBlockParser struct{}

// NewPIBlockParser returns a block parser for processing instructions.
func NewPIBlockParser() parser.BlockParser {
	return &piBlockParser{}
}

// Trigger returns the bytes that can start a PI block.
func (p *piBlockParser) Trigger() []byte {
	return []byte{'<'}
}

// Open attempts to open a ProcessingInstruction block.
func (p *piBlockParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	// Only accept PIs at the document root.
	if parent.Kind() != ast.KindDocument {
		return nil, parser.NoChildren
	}

	line, seg := reader.PeekLine()
	if line == nil {
		return nil, parser.NoChildren
	}

	// Allow up to 3 spaces of indentation.
	trimmed := bytes.TrimLeft(line, " ")
	indent := len(line) - len(trimmed)
	if indent > 3 {
		return nil, parser.NoChildren
	}

	if !bytes.HasPrefix(trimmed, piOpen) {
		return nil, parser.NoChildren
	}

	// Extract the name.
	rest := trimmed[2:]
	name := extractPINameBytes(rest)
	if len(name) == 0 {
		return nil, parser.NoChildren
	}

	node := &ProcessingInstruction{
		Name: string(name),
	}
	node.Lines().Append(seg)

	// Mark single-line PIs (e.g. <?foo?> or <?foo?> trailing) as
	// closed. The actual block close happens in Continue; this just
	// records the closure.
	trimmedRight := bytes.TrimRight(trimmed, " \t\r\n")
	if bytes.Contains(trimmedRight, piClose) {
		node.ClosureLine = seg
	}

	reader.AdvanceToEOL()
	return node, parser.NoChildren
}

// Continue checks whether the PI block should continue or close.
func (p *piBlockParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	pi := node.(*ProcessingInstruction)

	// Single-line PI was already closed in Open — stop immediately
	// without consuming the current line.
	if pi.HasClosure() {
		return parser.Close
	}

	line, seg := reader.PeekLine()
	if line == nil {
		return parser.Close
	}

	if bytes.Equal(bytes.TrimSpace(line), piClose) {
		pi.ClosureLine = seg
		reader.AdvanceToEOL()
		return parser.Close
	}

	pi.Lines().Append(seg)
	reader.AdvanceToEOL()
	return parser.Continue | parser.NoChildren
}

// Close is a no-op.
func (p *piBlockParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {}

// CanInterruptParagraph returns true (matches HTML block behavior).
func (p *piBlockParser) CanInterruptParagraph() bool {
	return true
}

// CanAcceptIndentedLine returns false.
func (p *piBlockParser) CanAcceptIndentedLine() bool {
	return false
}

var (
	piOpen  = []byte("<?")
	piClose = []byte("?>")
)

// extractPINameBytes returns the PI name from the bytes after "<?".
// The name is the substring up to the first whitespace or "?>".
func extractPINameBytes(b []byte) []byte {
	b = bytes.TrimRight(b, "\r\n")
	for i, c := range b {
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			return b[:i]
		}
		if c == '?' && i+1 < len(b) && b[i+1] == '>' {
			return b[:i]
		}
	}
	return b
}

// PIBlockParserPrioritized returns the PI parser with its priority for registration.
func PIBlockParserPrioritized() util.PrioritizedValue {
	return util.Prioritized(NewPIBlockParser(), 850)
}
