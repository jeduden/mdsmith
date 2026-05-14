// Package forbiddenparagraphstarts implements MDS055, which flags
// paragraphs whose plain text begins with any configured prefix.
package forbiddenparagraphstarts

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

// Rule flags paragraphs whose plain text begins with one of the
// configured prefixes. The prefix match is case-sensitive on the
// trimmed paragraph text.
type Rule struct {
	Starts []string
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS055" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "forbidden-paragraph-starts" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "prose" }

// EnabledByDefault implements rule.Defaultable. MDS055 is opt-in;
// teams enable it for prose rules ("avoid starting paragraphs with
// 'We'") or for per-section overrides through a schema.
func (r *Rule) EnabledByDefault() bool { return false }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if f.AST == nil || len(r.Starts) == 0 {
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
		text := strings.TrimLeft(mdtext.ExtractPlainText(para, f.Source), " \t")
		for _, prefix := range r.Starts {
			if prefix == "" {
				continue
			}
			if strings.HasPrefix(text, prefix) {
				diags = append(diags, lint.Diagnostic{
					File:     f.Path,
					Line:     astutil.ParagraphLine(para, f),
					Column:   1,
					RuleID:   r.ID(),
					RuleName: r.Name(),
					Severity: lint.Warning,
					Message: fmt.Sprintf(
						"paragraph starts with forbidden prefix %q",
						prefix,
					),
				})
				break
			}
		}
		return ast.WalkContinue, nil
	})
	return diags
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(s map[string]any) error {
	for k, v := range s {
		switch k {
		case "starts":
			ss, ok := settings.ToStringSlice(v)
			if !ok {
				return fmt.Errorf(
					"forbidden-paragraph-starts: starts must be a list of strings, got %T",
					v,
				)
			}
			r.Starts = ss
		default:
			return fmt.Errorf(
				"forbidden-paragraph-starts: unknown setting %q", k,
			)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"starts": []string{},
	}
}

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.Defaultable  = (*Rule)(nil)
)
