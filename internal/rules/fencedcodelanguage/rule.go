package fencedcodelanguage

import (
	"github.com/jeduden/tidymark/internal/lint"
	"github.com/jeduden/tidymark/internal/rule"
	"github.com/jeduden/tidymark/internal/rules/fencedcodestyle"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that fenced code blocks have a language tag.
type Rule struct{}

func (r *Rule) ID() string   { return "TM011" }
func (r *Rule) Name() string { return "fenced-code-language" }

func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic

	ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		fcb, ok := n.(*ast.FencedCodeBlock)
		if !ok {
			return ast.WalkContinue, nil
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
			line := fencedcodestyle.FenceOpenLine(f, fcb)
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     line,
				Column:   1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  "fenced code block should have a language tag",
			})
		}

		return ast.WalkContinue, nil
	})

	return diags
}
