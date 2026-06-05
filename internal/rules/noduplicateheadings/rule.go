package noduplicateheadings

import (
	"strconv"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/astutil"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that no two headings have the same text content.
type Rule struct{}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS005" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "no-duplicate-headings" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "heading" }

// Check implements rule.Rule. It delegates to the shared walk so a
// direct caller (the LSP, unit tests) sees the same node stream the
// engine's multiplexed dispatch feeds the visitor.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	return rule.WalkVisitor(r, f)
}

// NewNodeVisitor implements rule.NodeVisitorRule. The visitor carries
// the seen-text map across the walk's Heading nodes, so the engine can
// fold this rule's traversal into the one shared ast.Walk. A fresh
// visitor per file keeps the map from leaking across files.
func (r *Rule) NewNodeVisitor(_ *lint.File) rule.NodeVisitor {
	return &visitor{rule: r, seen: map[string]int{}}
}

// visitor is the per-file worker: seen maps heading text to the line of
// its first occurrence, so a later identical heading is flagged.
type visitor struct {
	rule *Rule
	seen map[string]int
}

// Kinds implements rule.NodeVisitor: only Heading nodes matter.
func (v *visitor) Kinds() []ast.NodeKind { return []ast.NodeKind{ast.KindHeading} }

// VisitNode implements rule.NodeVisitor. It mirrors the original
// ast.Walk callback exactly: skip on leaving, skip the reserved "..."
// wildcard, flag a repeat, otherwise record the first occurrence.
func (v *visitor) VisitNode(n ast.Node, entering bool, f *lint.File) []lint.Diagnostic {
	if !entering {
		return nil
	}
	heading, ok := n.(*ast.Heading)
	if !ok {
		return nil
	}

	text := astutil.HeadingText(heading, f.Source)
	if strings.TrimSpace(text) == "..." {
		// Reserved wildcard marker for required-structure prototypes.
		return nil
	}
	line := astutil.HeadingLine(heading, f)

	firstLine, exists := v.seen[text]
	if !exists {
		v.seen[text] = line
		return nil
	}
	return []lint.Diagnostic{{
		File:     f.Path,
		Line:     line,
		Column:   1,
		RuleID:   v.rule.ID(),
		RuleName: v.rule.Name(),
		Severity: lint.Warning,
		Message: "duplicate heading " + strconv.Quote(text) +
			" (first defined on line " + strconv.Itoa(firstLine) + ")",
	}}
}

var _ rule.NodeVisitorRule = (*Rule)(nil)
