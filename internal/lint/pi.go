package lint

import (
	"fmt"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// KindProcessingInstruction is the ast.NodeKind for ProcessingInstruction.
var KindProcessingInstruction = ast.NewNodeKind("ProcessingInstruction")

// ProcessingInstruction is a custom AST block node for XML processing
// instructions (<?name ... ?>). It replaces the default ast.HTMLBlock
// representation for PI syntax.
type ProcessingInstruction struct {
	ast.BaseBlock
	ClosureLine text.Segment
	Name        string // directive name from <?name
}

// Kind implements ast.Node.
func (n *ProcessingInstruction) Kind() ast.NodeKind {
	return KindProcessingInstruction
}

// IsRaw implements ast.Node. Processing instructions contain raw content.
func (n *ProcessingInstruction) IsRaw() bool {
	return true
}

// HasClosure reports whether the PI was closed with ?>. Mirrors ast.HTMLBlock.
func (n *ProcessingInstruction) HasClosure() bool {
	return n.ClosureLine.Start != n.ClosureLine.Stop
}

// Dump implements ast.Node.
func (n *ProcessingInstruction) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{
		"Name": fmt.Sprintf("%q", n.Name),
	}, nil)
}
