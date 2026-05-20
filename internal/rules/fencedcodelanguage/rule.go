package fencedcodelanguage

import (
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/fencepos"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that fenced code blocks have a language tag.
type Rule struct{}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS011" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "fenced-code-language" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "code" }

// Check implements rule.Rule. The per-block logic is pure and
// stateless, so it is expressed as CheckNode and the engine can fold
// this rule into one shared AST walk; a direct call still works via
// rule.WalkNodes.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	return rule.WalkNodes(r, f)
}

// CheckNode implements rule.NodeChecker.
func (r *Rule) CheckNode(n ast.Node, entering bool, f *lint.File) []lint.Diagnostic {
	if !entering {
		return nil
	}
	fcb, ok := n.(*ast.FencedCodeBlock)
	if !ok {
		return nil
	}

	hasLanguage := false
	if fcb.Info != nil {
		info := fcb.Info.Segment
		if info.Stop > info.Start {
			lang := f.Source[info.Start:info.Stop]
			if len(lang) > 0 {
				hasLanguage = true
			}
		}
	}

	if !hasLanguage {
		line := fencepos.OpenLine(f, fcb)
		return []lint.Diagnostic{{
			File:     f.Path,
			Line:     line,
			Column:   1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  "fenced code block should have a language tag",
		}}
	}
	return nil
}

var _ rule.NodeChecker = (*Rule)(nil)
