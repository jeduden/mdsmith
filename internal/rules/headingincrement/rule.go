package headingincrement

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

// Rule checks that heading levels only increment by one.
type Rule struct {
	Placeholders []string // placeholder tokens to treat as opaque
}

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

		// Check if this heading's text matches a configured placeholder.
		// Placeholder headings skip the increment diagnostic but still
		// update prevLevel so subsequent headings track correctly.
		isPlaceholder := len(r.Placeholders) > 0 &&
			placeholders.ContainsBodyToken(astutil.HeadingText(heading, f.Source), r.Placeholders)

		if prevLevel == 0 {
			// First heading: should be h1
			if level > 1 && !isPlaceholder {
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
		} else if level > prevLevel+1 && !isPlaceholder {
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

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(s map[string]any) error {
	for k, v := range s {
		switch k {
		case "placeholders":
			toks, ok := settings.ToStringSlice(v)
			if !ok {
				return fmt.Errorf("heading-increment: placeholders must be a list of strings, got %T", v)
			}
			if err := placeholders.Validate(toks); err != nil {
				return fmt.Errorf("heading-increment: %w", err)
			}
			r.Placeholders = toks
		default:
			return fmt.Errorf("heading-increment: unknown setting %q", k)
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

// ListMergeMode implements rule.ListMerger: placeholders concatenate
// across config layers.
func (r *Rule) ListMergeMode(key string) rule.ListMergeMode {
	if key == "placeholders" {
		return rule.ListAppend
	}
	return rule.ListReplace
}

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.ListMerger   = (*Rule)(nil)
)
