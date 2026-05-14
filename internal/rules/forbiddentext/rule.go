// Package forbiddentext implements MDS056, which flags paragraphs whose
// plain text contains any configured substring.
package forbiddentext

import (
	"fmt"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/astutil"
	"github.com/jeduden/mdsmith/internal/rules/settings"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule flags paragraphs whose plain text contains one of the configured
// substrings. Each match emits a diagnostic at the paragraph's start
// line so the per-scope override in plan 146 keeps only diagnostics
// that fall inside the configured section's line range.
type Rule struct {
	Contains []string
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS056" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "forbidden-text" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "prose" }

// EnabledByDefault implements rule.Defaultable.
func (r *Rule) EnabledByDefault() bool { return false }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if f.AST == nil || len(r.Contains) == 0 {
		return nil
	}
	var diags []lint.Diagnostic
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		para, ok := n.(*ast.Paragraph)
		if !ok {
			return ast.WalkContinue, nil
		}
		if astutil.IsTable(para, f) {
			return ast.WalkContinue, nil
		}
		text := mdtext.ExtractPlainText(para, f.Source)
		line := astutil.ParagraphLine(para, f)
		for _, sub := range r.Contains {
			if sub == "" {
				continue
			}
			if !strings.Contains(text, sub) {
				continue
			}
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     line,
				Column:   1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message: fmt.Sprintf(
					"paragraph contains forbidden text %q", sub,
				),
			})
		}
		return ast.WalkContinue, nil
	})
	return diags
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(s map[string]any) error {
	for k, v := range s {
		switch k {
		case "contains":
			ss, ok := settings.ToStringSlice(v)
			if !ok {
				return fmt.Errorf(
					"forbidden-text: contains must be a list of strings, got %T",
					v,
				)
			}
			r.Contains = ss
		default:
			return fmt.Errorf(
				"forbidden-text: unknown setting %q", k,
			)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"contains": []string{},
	}
}

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.Defaultable  = (*Rule)(nil)
)
