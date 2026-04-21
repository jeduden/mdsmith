package headingincrement

import (
	"fmt"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/astutil"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that heading levels only increment by one.
type Rule struct{}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS003" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "heading-increment" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "heading" }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic
	prevLevel := 0

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		heading, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}

		level := heading.Level
		if prevLevel == 0 {
			// First heading: should be h1
			if level > 1 {
				line := astutil.HeadingLine(heading, f)
				diags = append(diags, lint.Diagnostic{
					File:     f.Path,
					Line:     line,
					Column:   1,
					RuleID:   r.ID(),
					RuleName: r.Name(),
					Severity: lint.Warning,
					Message:  fmt.Sprintf("first heading level should be 1, got %d", level),
				})
			}
		} else if level > prevLevel+1 {
			line := astutil.HeadingLine(heading, f)
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     line,
				Column:   1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message: fmt.Sprintf("heading level incremented from %d to %d (expected %d)",
					prevLevel, level, prevLevel+1),
			})
		}

		prevLevel = level
		return ast.WalkContinue, nil
	})

	return diags
}
