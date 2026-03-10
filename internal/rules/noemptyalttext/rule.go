package noemptyalttext

import (
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that images have non-empty alt text for accessibility.
type Rule struct{}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS032" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "no-empty-alt-text" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "accessibility" }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		img, ok := n.(*ast.Image)
		if !ok {
			return ast.WalkContinue, nil
		}

		alt := string(img.Text(f.Source))
		if strings.TrimSpace(alt) == "" {
			line := imageLine(img, f)
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     line,
				Column:   1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  "image has empty alt text",
			})
		}

		return ast.WalkContinue, nil
	})

	return diags
}

func imageLine(img *ast.Image, f *lint.File) int {
	// Try child text nodes first for precise position.
	for c := img.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			return f.LineOfOffset(t.Segment.Start)
		}
	}
	// Walk up ancestors to find a block node with line info.
	for p := img.Parent(); p != nil; p = p.Parent() {
		lines := p.Lines()
		if lines != nil && lines.Len() > 0 {
			return f.LineOfOffset(lines.At(0).Start)
		}
	}
	return 1
}
