package noemphasisasheading

import (
	"fmt"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/placeholders"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/astutil"
	"github.com/jeduden/mdsmith/internal/rules/settings"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that emphasis/strong emphasis is not used as a heading substitute.
// A paragraph whose only content is emphasis or strong emphasis is flagged.
type Rule struct {
	Placeholders []string // placeholder tokens to treat as opaque
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS018" }

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

		// If the emphasis text contains a configured placeholder token,
		// treat it as opaque and suppress the diagnostic.
		if len(r.Placeholders) > 0 {
			var emphText string
			_ = ast.Walk(firstChild, func(inner ast.Node, entering bool) (ast.WalkStatus, error) {
				if !entering {
					return ast.WalkContinue, nil
				}
				if t, ok2 := inner.(*ast.Text); ok2 {
					emphText += string(t.Segment.Value(f.Source))
				}
				return ast.WalkContinue, nil
			})
			if placeholders.ContainsBodyToken(emphText, r.Placeholders) {
				return ast.WalkContinue, nil
			}
		}

		line := astutil.ParagraphLine(para, f)
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

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(s map[string]any) error {
	for k, v := range s {
		switch k {
		case "placeholders":
			toks, ok := settings.ToStringSlice(v)
			if !ok {
				return fmt.Errorf("no-emphasis-as-heading: placeholders must be a list of strings, got %T", v)
			}
			if err := placeholders.Validate(toks); err != nil {
				return fmt.Errorf("no-emphasis-as-heading: %w", err)
			}
			r.Placeholders = toks
		default:
			return fmt.Errorf("no-emphasis-as-heading: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"placeholders": []string{},
	}
}

var _ rule.Configurable = (*Rule)(nil)
