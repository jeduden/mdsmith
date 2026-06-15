package rule

import (
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
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

// KindScopedChecker is an optional refinement of NodeChecker for rules
// whose CheckNode only ever reacts to a fixed set of node kinds, and
// only on the entering visit. The engine's shared walk dispatches such
// a rule exclusively for nodes of those kinds (entering only), instead
// of calling CheckNode for every node in the tree — with dozens of
// NodeChecker rules enabled, the per-node interface calls that
// immediately return nil dominated the walk's cost.
//
// Contract: EnteringKinds must return every kind CheckNode can emit a
// diagnostic for, the result must be constant for the life of the rule
// (the dispatch table is built from it once per file), and CheckNode
// must not depend on being called for other kinds or for leaving
// visits. A rule that needs exit visits or dynamic kind interest
// should implement plain NodeChecker instead.
type KindScopedChecker interface {
	NodeChecker
	EnteringKinds() []ast.NodeKind
}

// BlockChecker is an optional capability for a NodeChecker rule whose
// per-node logic depends only on a block's kind and source line span —
// never on the inline node tree under it. Such a rule can run from the
// Layer 0 block scan (lint.Layer0) without a goldmark parse: the engine
// drives CheckBlock over the block spans of a nil-AST File instead of
// CheckNode over an AST.
//
// Contract: for every File, CheckBlock must return precisely the
// diagnostics CheckNode would over the same document — same line,
// column, message, severity — so the two paths are byte-identical
// (the layer0 equivalence gate enforces this across the corpus).
// BlockKinds must return every lint.BlockKind CheckBlock can emit a
// diagnostic for; the engine dispatches a span to a rule only when the
// span's kind is in that set, exactly as KindScopedChecker.EnteringKinds
// scopes the AST walk. The result must be constant for the life of the
// rule.
type BlockChecker interface {
	NodeChecker
	// CheckBlock is invoked once per block span whose Kind is in
	// BlockKinds(), in document order. It reads the span's kind and
	// 1-based inclusive line range plus f.Lines; it must not touch
	// f.AST (which is nil on the Layer 0 path).
	CheckBlock(span lint.BlockSpan, f *lint.File) []lint.Diagnostic
	// BlockKinds returns the block kinds CheckBlock reacts to.
	BlockKinds() []lint.BlockKind
}

// WalkBlocks runs r.CheckBlock over the Layer 0 block scan of f,
// dispatching only the spans whose Kind is in r.BlockKinds(), in
// document order. A BlockChecker's standalone Check delegates here for
// the nil-AST path so direct callers (unit tests) get behaviour
// identical to the engine's block dispatch. A nil File returns nil.
func WalkBlocks(r BlockChecker, f *lint.File) []lint.Diagnostic {
	if f == nil {
		return nil
	}
	kinds := r.BlockKinds()
	var diags []lint.Diagnostic
	for _, span := range lint.Layer0(f).BlockSpans {
		if !blockKindInSet(span.Kind, kinds) {
			continue
		}
		diags = append(diags, r.CheckBlock(span, f)...)
	}
	return diags
}

// blockKindInSet reports whether k is in kinds. The set is tiny (a rule
// reacts to one or two block kinds), so a linear scan beats a map and
// allocates nothing.
func blockKindInSet(k lint.BlockKind, kinds []lint.BlockKind) bool {
	for _, want := range kinds {
		if k == want {
			return true
		}
	}
	return false
}

// WalkNodes runs r.CheckNode over a single ast.Walk of f. A
// NodeChecker's standalone Check delegates here so direct callers
// (the LSP, unit tests) get behaviour identical to the engine's
// multiplexed dispatch, which feeds CheckNode the same node stream.
// Files with a nil AST short-circuit to no diagnostics; the engine
// never produces such files, but unit tests construct
// `&lint.File{}` literals to exercise rule guards.
func WalkNodes(r NodeChecker, f *lint.File) []lint.Diagnostic {
	if f == nil || f.AST == nil {
		return nil
	}
	var diags []lint.Diagnostic
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		diags = append(diags, r.CheckNode(n, entering, f)...)
		return ast.WalkContinue, nil
	})
	return diags
}
