package rule

import (
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

// NodeChecker is an optional capability for a rule whose Check is a
// per-node pass: it reacts to each AST node as the walk reaches it and
// does not depend on skipping a subtree or stopping the walk for
// correctness. A NodeChecker may thread state from one node to the next
// (heading-increment compares each heading against the previous one),
// but only within a single File, and only by implementing FileResetter
// so the engine can clear that state at each file boundary — a
// NodeChecker never carries state from one File to the next, and never
// from one Check call to the next. The engine drives ONE shared
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

// InlineChecker is an optional capability for a NodeChecker rule whose
// Check handles the parse-skipped path (f.AST nil) itself, by reading the
// shared run-grouped inline parse (lint.InlineBlocks) instead of the tree.
// The engine routes such a rule to its own Check on a nil-AST File rather
// than dropping it (a NodeChecker that navigates the tree cannot run on a
// nil AST) or forcing it onto the block-span dispatch (a rule reacting to
// inline link/image markup needs the parsed inline nodes, not a block's
// line span).
//
// Contract: on a nil-AST File, Check must return precisely the diagnostics
// the rule's NodeChecker path would over the same document — same line,
// column, message, severity — so the two paths are byte-identical (the
// corpus and gate equivalence harnesses enforce this). InlineCapable is a
// pure marker; its result must be constant for the life of the rule.
type InlineChecker interface {
	NodeChecker
	// InlineCapable reports that this rule's Check serves the nil-AST
	// path from lint.InlineBlocks. It is a marker only — the dispatch
	// decision is made from it, no behaviour hangs off the value.
	InlineCapable() bool
}

// LinesChecker is an optional capability for a NodeChecker rule whose Check
// handles the parse-skipped path (f.AST nil) itself by re-deriving block
// structure from f.Lines — directly, or through the Layer 0 block scan
// derived from them — rather than from the tree. The list rules (MDS014,
// MDS016, MDS045, MDS046, MDS061) are NodeCheckers on the parsed path but
// cannot walk a nil tree, and they do not reduce to a single block-span the
// way a fence rule does (a list's verdict spans every item line and its
// nesting), so neither the AST walk nor the block-span dispatch can drive
// them on a skipped File; heading-increment (MDS003) qualifies for a
// different reason — its block-span verdict is only equivalent while no
// placeholder vocabulary is configured, which BlockChecker's
// every-File contract does not allow. This marker tells the engine to route
// such a rule to its own Check instead of dropping it.
//
// Contract, identical to InlineChecker: on a nil-AST File, Check must
// return precisely the diagnostics the rule's NodeChecker path would over
// the same document — same line, column, message, severity. The listscan
// corpus equivalence test plus the engine Layer-0 gate harness enforce it.
// LinesCapable is a pure marker; its result is constant for the rule.
type LinesChecker interface {
	NodeChecker
	// LinesCapable reports that this rule's Check serves the nil-AST path
	// from f.Lines. It is a marker only — the dispatch decision is made
	// from it, no behaviour hangs off the value.
	LinesCapable() bool
}

// FileResetter is an optional capability for a NodeChecker rule that carries
// per-file state in its struct fields. BeginFile is called once per file
// before any dispatch begins, giving the rule a chance to reset any state
// left over from the previous file. Every dispatch path honours it: the
// engine's runNodeCheckers (via prepareNodeCheckers) on the AST walk, its
// runBlockCheckers on the Layer 0 block-span path, and WalkNodes/WalkBlocks
// so unit tests and the LSP observe the same reset contract. Implementing
// this interface is the correct way to add cross-heading (or cross-node)
// state to a KindScopedChecker without the stale-state bug that arises from
// worker-instance reuse across files.
//
// Concurrency: because the state lives on the rule instance, a FileResetter
// must never be shared between goroutines. checker.ConfigureEnabledRules
// enforces that — it hands every configured rule list its own clone of a
// FileResetter rule, so two concurrent callers passing the same rule slice
// still write to different instances. A rule implementing this interface
// should nevertheless keep CheckNode safe against a missing BeginFile (lazily
// initialising a map field rather than assuming BeginFile allocated it), so a
// future dispatch path that forgets the reset degrades instead of panicking.
type FileResetter interface {
	NodeChecker
	// BeginFile is called exactly once per File, before CheckNode (or
	// CheckBlock) is first called for that File. It must reset all per-file
	// state so no values from a previously processed File persist into this
	// one.
	BeginFile(f *lint.File)
}

// WalkBlocks runs r.CheckBlock over the Layer 0 block scan of f,
// dispatching only the spans whose Kind is in r.BlockKinds(), in
// document order. A BlockChecker's standalone Check delegates here for
// the nil-AST path so direct callers (unit tests) get behaviour
// identical to the engine's block dispatch. A nil File returns nil.
//
// If r implements FileResetter, WalkBlocks calls r.BeginFile(f) before the
// first span, matching runBlockCheckers.
func WalkBlocks(r BlockChecker, f *lint.File) []lint.Diagnostic {
	if f == nil {
		return nil
	}
	if fr, ok := r.(FileResetter); ok {
		fr.BeginFile(f)
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
//
// If r implements FileResetter, WalkNodes calls r.BeginFile(f) before
// the walk so per-file state is reset on every standalone Check call,
// matching the engine's runNodeCheckers contract.
//
// If r implements KindScopedChecker, WalkNodes applies the same scoping
// the engine's dispatchScoped applies: CheckNode is called only on
// entering visits of nodes whose kind is in EnteringKinds(). Without it a
// standalone Check on a kind-scoped rule pays two CheckNode calls for
// every node in the document that immediately return nil — the exact cost
// KindScopedChecker exists to remove.
func WalkNodes(r NodeChecker, f *lint.File) []lint.Diagnostic {
	if f == nil || f.AST == nil {
		return nil
	}
	if fr, ok := r.(FileResetter); ok {
		fr.BeginFile(f)
	}
	ks, scoped := r.(KindScopedChecker)
	var kinds []ast.NodeKind
	if scoped {
		// Read once: EnteringKinds is contractually constant, and the
		// engine likewise reads it once per file when building its table.
		kinds = ks.EnteringKinds()
	}
	var diags []lint.Diagnostic
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if scoped && (!entering || !nodeKindInSet(n.Kind(), kinds)) {
			return ast.WalkContinue, nil
		}
		diags = append(diags, r.CheckNode(n, entering, f)...)
		return ast.WalkContinue, nil
	})
	return diags
}

// nodeKindInSet reports whether k is in kinds. The set is tiny (a
// kind-scoped rule declares one or two kinds), so a linear scan beats a
// map and allocates nothing — the same trade-off blockKindInSet makes.
func nodeKindInSet(k ast.NodeKind, kinds []ast.NodeKind) bool {
	for _, want := range kinds {
		if k == want {
			return true
		}
	}
	return false
}
