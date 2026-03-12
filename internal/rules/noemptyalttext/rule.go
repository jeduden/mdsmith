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

		alt := imageAltText(img, f)
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

func imageAltText(img *ast.Image, f *lint.File) string {
	var b strings.Builder
	collectText(&b, img, f.Source)
	return b.String()
}

func collectText(b *strings.Builder, n ast.Node, source []byte) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			b.Write(t.Segment.Value(source))
		} else {
			collectText(b, c, source)
		}
	}
}

// isInlineNode returns true for inline AST nodes whose Lines() panics.
func isInlineNode(n ast.Node) bool {
	switch n.(type) {
	case *ast.Text, *ast.String, *ast.CodeSpan, *ast.Emphasis,
		*ast.Link, *ast.Image, *ast.AutoLink, *ast.RawHTML:
		return true
	}
	return false
}

func imageLine(img *ast.Image, f *lint.File) int {
	// Try child text nodes first for precise position.
	line := firstTextLine(img, f)
	if line > 0 {
		return line
	}
	// Walk up ancestors, skipping inline nodes whose Lines() panics.
	for p := img.Parent(); p != nil; p = p.Parent() {
		if isInlineNode(p) {
			continue
		}
		lines := p.Lines()
		if lines != nil && lines.Len() > 0 {
			return f.LineOfOffset(lines.At(0).Start)
		}
	}
	return 1
}

func firstTextLine(n ast.Node, f *lint.File) int {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			return f.LineOfOffset(t.Segment.Start)
		}
		if line := firstTextLine(c, f); line > 0 {
			return line
		}
	}
	return 0
}
