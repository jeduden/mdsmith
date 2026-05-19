package rule

import (
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/yuin/goldmark/ast"
)

// NodeChecker is an optional capability for a rule whose Check is a
// pure per-node pass: it inspects each AST node independently, keeps
// no state across nodes, and does not depend on skipping a subtree or
// stopping the walk for correctness. The engine drives ONE shared
// ast.Walk for every enabled NodeChecker instead of each rule
// re-walking the whole tree (goldmark walkHelper was ~44% cumulative
// with N per-rule walks). The engine still appends each rule's
// diagnostics as one contiguous group in rule order, so the result
// is byte-identical to running each rule's Check sequentially.
type NodeChecker interface {
	Rule
	// CheckNode is invoked for every node, once entering and (for
	// container nodes) once leaving, in the exact pre-order
	// goldmark ast.Walk uses. It must return precisely the
	// diagnostics the rule's own ast.Walk Check would, and must not
	// rely on ast.WalkSkipChildren or ast.WalkStop.
	CheckNode(n ast.Node, entering bool, f *lint.File) []lint.Diagnostic
}

// WalkNodes runs r.CheckNode over a single ast.Walk of f. A
// NodeChecker's standalone Check delegates here so direct callers
// (the LSP, unit tests) get behaviour identical to the engine's
// multiplexed dispatch, which feeds CheckNode the same node stream.
func WalkNodes(r NodeChecker, f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		diags = append(diags, r.CheckNode(n, entering, f)...)
		return ast.WalkContinue, nil
	})
	return diags
}
