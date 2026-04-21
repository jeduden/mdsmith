package notrailingpunctuation

import (
	"fmt"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/astutil"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that heading text does not end with trailing punctuation.
type Rule struct{}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS017" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "no-trailing-punctuation-in-heading" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "heading" }

// flaggedPunctuation contains the punctuation characters that are not allowed
// at the end of a heading.
const flaggedPunctuation = ".,;:!"

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		heading, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}

		text := astutil.HeadingText(heading, f.Source)
		text = strings.TrimSpace(text)
		if len(text) == 0 {
			return ast.WalkContinue, nil
		}
		if text == "..." {
			// Reserved wildcard marker for required-structure prototypes.
			return ast.WalkContinue, nil
		}

		lastChar := text[len(text)-1]
		if strings.ContainsRune(flaggedPunctuation, rune(lastChar)) {
			line := astutil.HeadingLine(heading, f)
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     line,
				Column:   1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  fmt.Sprintf("heading should not end with punctuation %q", string(lastChar)),
			})
		}

		return ast.WalkContinue, nil
	})

	return diags
}
