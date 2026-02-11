package noemphasisasheading

import (
	"github.com/jeduden/tidymark/internal/lint"
	"github.com/jeduden/tidymark/internal/rule"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that emphasis/strong emphasis is not used as a heading substitute.
// A paragraph whose only content is emphasis or strong emphasis is flagged.
type Rule struct{}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "TM018" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "no-emphasis-as-heading" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "heading" }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		para, ok := n.(*ast.Paragraph)
		if !ok {
			return ast.WalkContinue, nil
		}

		// Check if the paragraph has exactly one child that is emphasis or strong
		firstChild := para.FirstChild()
		if firstChild == nil {
			return ast.WalkContinue, nil
		}

		// Must be the only child
		if firstChild.NextSibling() != nil {
			return ast.WalkContinue, nil
		}

		// Check if it's emphasis or strong emphasis
		_, isEmphasis := firstChild.(*ast.Emphasis)
		if !isEmphasis {
			return ast.WalkContinue, nil
		}

		line := paragraphLine(para, f)
		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     line,
			Column:   1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  "emphasis used instead of a heading",
		})

		return ast.WalkContinue, nil
	})

	return diags
}

func paragraphLine(para *ast.Paragraph, f *lint.File) int {
	lines := para.Lines()
	if lines.Len() > 0 {
		return f.LineOfOffset(lines.At(0).Start)
	}
	return 1
}
