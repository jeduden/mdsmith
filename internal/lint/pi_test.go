package lint

import (
	"github.com/yuin/goldmark/ast"

	"github.com/jeduden/mdsmith/internal/pi"
)

// findPINodes returns all ProcessingInstruction nodes in the AST,
// searching the full tree recursively. The exhaustive PI grammar
// tests live with the canonical parser in pkg/markdown; this helper
// backs the lint-level NewFile integration smoke
// (TestNewFile_MultiPIs).
func findPINodes(root ast.Node) []*pi.ProcessingInstruction {
	var nodes []*pi.ProcessingInstruction
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if node, ok := n.(*pi.ProcessingInstruction); ok {
			nodes = append(nodes, node)
		}
		return ast.WalkContinue, nil
	})
	return nodes
}
