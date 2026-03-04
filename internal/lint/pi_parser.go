package lint

import (
	"strings"

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
	trimmed := strings.TrimLeft(string(line), " ")
	indent := len(line) - len(trimmed)
	if indent > 3 {
		return nil, parser.NoChildren
	}

	if !strings.HasPrefix(trimmed, "<?") {
		return nil, parser.NoChildren
	}

	// Extract the name.
	rest := trimmed[2:]
	name := extractPIName(rest)
	if name == "" {
		return nil, parser.NoChildren
	}

	node := &ProcessingInstruction{
		Name: name,
	}
	node.Lines().Append(seg)

	// Mark single-line PIs (e.g. <?foo?> or <?foo?> trailing) as
	// closed. The actual block close happens in Continue; this just
	// records the closure.
	trimmedRight := strings.TrimRight(trimmed, " \t\r\n")
	if strings.Contains(trimmedRight, "?>") {
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

	trimmed := strings.TrimSpace(string(line))
	if trimmed == "?>" {
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

// extractPIName returns the PI name from the text after "<?".
// The name is the substring up to the first whitespace or "?>".
func extractPIName(s string) string {
	s = strings.TrimRight(s, "\r\n")

	var name strings.Builder
	for i, ch := range s {
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			break
		}
		if ch == '?' && i+1 < len(s) && s[i+1] == '>' {
			break
		}
		name.WriteRune(ch)
	}
	return name.String()
}

// PIBlockParserPrioritized returns the PI parser with its priority for registration.
func PIBlockParserPrioritized() util.PrioritizedValue {
	return util.Prioritized(NewPIBlockParser(), 850)
}
