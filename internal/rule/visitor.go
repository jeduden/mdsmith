package rule

import (
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/yuin/goldmark/ast"
)

// NodeVisitor is the per-walk worker for a stateful per-node rule. It
// is created fresh for each file (see NodeVisitorRule.NewNodeVisitor),
// so it may carry state across the nodes of one walk — a running
// heading level, a seen-text map — without leaking across files or
// goroutines. The engine drives ONE shared ast.Walk and routes only
// the declared kinds to each visitor.
//
// NodeVisitor is the stateful sibling of NodeChecker. NodeChecker suits
// a pure, stateless per-node pass and is shown every node; NodeVisitor
// suits a rule whose original Check carried a value across the walk
// (e.g. heading-increment's prevLevel, no-duplicate-headings' seen
// map), which a stateless callback on a shared rule instance could not
// express safely under intra-file parallelism.
type NodeVisitor interface {
	// Kinds returns the node kinds VisitNode should be invoked for. A
	// nil or empty slice means "every kind" (matching a bare ast.Walk
	// with no type switch). Declaring the exact kinds lets the engine
	// skip the visitor for nodes it does not care about, which is the
	// whole point of folding N type-switching walks into one.
	Kinds() []ast.NodeKind
	// VisitNode is invoked for every node of a declared kind, once
	// entering and (for container nodes) once leaving, in the exact
	// pre-order ast.Walk uses. It must return precisely the
	// diagnostics the rule's own ast.Walk Check would, and must not
	// rely on ast.WalkSkipChildren or ast.WalkStop on the shared walk.
	VisitNode(n ast.Node, entering bool, f *lint.File) []lint.Diagnostic
}

// NodeVisitorRule is an optional capability for a rule that drives the
// shared walk through a fresh per-file NodeVisitor. The engine calls
// NewNodeVisitor once per file and feeds the resulting visitor the
// shared node stream; the rule's own standalone Check delegates to
// WalkVisitor so direct callers (the LSP, unit tests) get behaviour
// identical to the engine's multiplexed dispatch.
type NodeVisitorRule interface {
	Rule
	// NewNodeVisitor returns a fresh visitor for one walk of f.
	// Returning nil means the rule has nothing to do for this file
	// (e.g. an unconfigured opt-in rule) and contributes no
	// diagnostics. The fresh-per-file contract is what keeps a
	// stateful visitor race-clean: its mutable state never outlives a
	// single walk and is never shared across goroutines.
	NewNodeVisitor(f *lint.File) NodeVisitor
}

// WalkVisitor runs r's per-file visitor over a single ast.Walk of f,
// invoking VisitNode only for the kinds the visitor declares. A
// NodeVisitorRule's standalone Check delegates here so the engine's
// multiplexed dispatch and the direct Check path produce identical
// output. Files with a nil AST (and a nil visitor) short-circuit to no
// diagnostics; the engine never produces a nil-AST file, but unit tests
// construct `&lint.File{}` literals to exercise rule guards.
func WalkVisitor(r NodeVisitorRule, f *lint.File) []lint.Diagnostic {
	if f == nil || f.AST == nil {
		return nil
	}
	v := r.NewNodeVisitor(f)
	if v == nil {
		return nil
	}
	want := kindSet(v.Kinds())
	var diags []lint.Diagnostic
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if wantsKind(want, n.Kind()) {
			diags = append(diags, v.VisitNode(n, entering, f)...)
		}
		return ast.WalkContinue, nil
	})
	return diags
}

// kindSet turns a visitor's declared kinds into a lookup set. A nil or
// empty slice yields a nil set, which wantsKind reads as "every kind".
func kindSet(kinds []ast.NodeKind) map[ast.NodeKind]struct{} {
	if len(kinds) == 0 {
		return nil
	}
	set := make(map[ast.NodeKind]struct{}, len(kinds))
	for _, k := range kinds {
		set[k] = struct{}{}
	}
	return set
}

// wantsKind reports whether a node of kind k should be routed to a
// visitor whose declared-kind set is want. A nil set means the visitor
// declared no kinds, i.e. it wants every node.
func wantsKind(want map[ast.NodeKind]struct{}, k ast.NodeKind) bool {
	if want == nil {
		return true
	}
	_, ok := want[k]
	return ok
}
