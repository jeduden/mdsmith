package noduplicateheadings

import (
	"strconv"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/astutil"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that no two headings have the same text content.
type Rule struct {
	// seen is per-file state: the heading texts observed so far within the
	// current file's walk, mapping text to its 1-based first-occurrence line.
	// Reset by BeginFile before each file's CheckNode sequence so state never
	// leaks from one file to the next when a worker clone processes multiple
	// files. BeginFile always allocates a fresh map (rather than clearing the
	// existing one) so that even when two worker clones share the same initial
	// seen pointer (from a shallow CloneInstance call on an already-walked
	// instance), each clone gets its own independent map once its BeginFile
	// runs — preventing the concurrent map write data race that clear would
	// cause on the shared backing store. The instance itself is never shared:
	// checker.ConfigureEnabledRules clones every rule.FileResetter so each
	// configured rule list owns its own.
	seen map[string]int
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS005" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "no-duplicate-headings" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "heading" }

// Check implements rule.Rule. On a nil-AST File it calls checkFromInline
// (the run-grouped inline parse path). On a parsed File it delegates to
// rule.WalkNodes, which calls BeginFile to reset per-file state and then
// dispatches CheckNode for every node via a single ast.Walk.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if f != nil && f.AST == nil {
		return r.checkFromInline(f)
	}
	return rule.WalkNodes(r, f)
}

// BeginFile implements rule.FileResetter. It replaces r.seen with a fresh
// map so that heading texts from a previously processed file do not
// contaminate the current file's duplicate check, and so that two worker
// clones that were shallow-copied from the same source instance never share
// a map backing store: each clone calls BeginFile before its first CheckNode
// and immediately gets an independent map, eliminating the concurrent-write
// data race that clear would trigger on a shared map. Called by
// rule.WalkNodes (for standalone Check callers) and by the engine's
// runNodeCheckers (for the shared walk path) before the first CheckNode call
// for each File.
func (r *Rule) BeginFile(_ *lint.File) {
	r.seen = make(map[string]int, 4)
}

// InlineCapable implements rule.InlineChecker. It returns true to tell the
// engine that this rule's Check method handles the nil-AST parse-skip path
// itself (via checkFromInline, which walks the shared run-grouped inline
// parse instead of the AST). Without this marker, classifySlot would drop
// the rule on nil-AST files because it is a NodeChecker but not a
// BlockChecker — silently producing no diagnostics where the rule should run.
func (r *Rule) InlineCapable() bool { return true }

// enteringKinds is the static node-kind interest CheckNode declares via
// rule.KindScopedChecker; package-level so EnteringKinds returns it without
// allocating.
var enteringKinds = []ast.NodeKind{ast.KindHeading}

// EnteringKinds implements rule.KindScopedChecker: CheckNode only reacts
// to heading nodes, entering visits only.
func (r *Rule) EnteringKinds() []ast.NodeKind { return enteringKinds }

// CheckNode implements rule.NodeChecker. It applies the first-seen-wins
// duplicate check to one heading, using r.seen to track which heading
// texts have already appeared in this file's walk. BeginFile initialises
// r.seen before the first CheckNode call for each File.
func (r *Rule) CheckNode(n ast.Node, entering bool, f *lint.File) []lint.Diagnostic {
	if !entering {
		return nil
	}
	heading, ok := n.(*ast.Heading)
	if !ok {
		return nil
	}
	if r.seen == nil {
		// Defensive: verdict writes into the map, and a nil map write is a
		// non-recoverable runtime panic. Every engine dispatch path calls
		// BeginFile first, but a direct CheckNode caller (a unit test, or a
		// future dispatch path that forgets the reset) would otherwise kill
		// the process instead of simply starting from an empty set.
		r.seen = make(map[string]int, 4)
	}
	text := astutil.HeadingTextCached(f, heading)
	line := astutil.HeadingLine(heading, f)
	if d, ok := r.verdict(f, text, line, r.seen); ok {
		return []lint.Diagnostic{d}
	}
	return nil
}

// checkFromInline runs the duplicate scan over the re-parsed inline runs
// for the nil-AST path. base maps each heading's run-local segment offsets
// back to the document so the heading text and line match the AST walk.
func (r *Rule) checkFromInline(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic
	seen := make(map[string]int) // local map — nil-AST path uses no struct state
	lint.WalkInlineNodes(f, func(n ast.Node, base int) {
		heading, ok := n.(*ast.Heading)
		if !ok {
			return
		}
		text := astutil.HeadingTextBaseCached(f, heading, base)
		line := astutil.HeadingLineBase(heading, f, base)
		if d, ok := r.verdict(f, text, line, seen); ok {
			diags = append(diags, d)
		}
	})
	return diags
}

// verdict applies the first-seen-wins duplicate check to one heading. text
// is the heading's flattened content, line its 1-based source line, and
// seen the running first-occurrence map (mutated to record a new text). It
// returns the diagnostic for a repeat heading, or ok == false when the
// heading is the first of its text or the reserved `...` wildcard.
func (r *Rule) verdict(f *lint.File, text string, line int, seen map[string]int) (lint.Diagnostic, bool) {
	if strings.TrimSpace(text) == "..." {
		// Reserved wildcard marker for required-structure prototypes.
		return lint.Diagnostic{}, false
	}
	if firstLine, exists := seen[text]; exists {
		return lint.Diagnostic{
			File:     f.Path,
			Line:     line,
			Column:   1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message: "duplicate heading " + strconv.Quote(text) +
				" (first defined on line " + strconv.Itoa(firstLine) + ")",
		}, true
	}
	seen[text] = line
	return lint.Diagnostic{}, false
}

var (
	_ rule.KindScopedChecker = (*Rule)(nil)
	_ rule.FileResetter      = (*Rule)(nil)
	_ rule.InlineChecker     = (*Rule)(nil)
)
